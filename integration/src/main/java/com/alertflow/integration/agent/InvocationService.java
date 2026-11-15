package com.alertflow.integration.agent;

import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.time.Duration;
import java.util.ArrayList;
import java.util.HexFormat;
import java.util.List;
import java.util.Map;
import java.util.TreeMap;
import java.util.concurrent.TimeUnit;

import com.alertflow.gen.integration.v1.InvokeAgentRequest;
import com.alertflow.gen.integration.v1.InvokeAgentResponse;
import com.alertflow.integration.bridge.HeaderForwarding;
import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.google.protobuf.Struct;
import com.google.protobuf.util.JsonFormat;
import io.grpc.Context;
import io.grpc.Deadline;
import io.grpc.Metadata;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

public class InvocationService {
    public static final String SKILL = "knowledge-retrieval";
    private static final Logger log = LoggerFactory.getLogger(InvocationService.class);
    // 服务端允许的单次 A2A 调用上限；入站 Deadline 更短时取更短。
    private static final long MAX_TIMEOUT_MS = env("INTEGRATION_A2A_MAX_TIMEOUT_MS", 60_000);
    private final InvocationStore store;
    private final AgentRegistry registry;
    private final KnowledgeClient knowledge;
    private final CallGovernance governance;
    private final ObjectMapper json = new ObjectMapper();

    public InvocationService(InvocationStore store, AgentRegistry registry, KnowledgeClient knowledge, CallGovernance governance) {
        this.store = store;
        this.registry = registry;
        this.knowledge = knowledge;
        this.governance = governance;
    }

    public InvokeAgentResponse invoke(InvokeAgentRequest request) throws Exception {
        String caller = caller();
        if (caller.isBlank() || request.getInvocationId().isBlank() || !SKILL.equals(request.getSkillId())) {
            return failed(request, "INVALID_ARGUMENT", "caller, invocation_id and skill are required");
        }
        if (!"knowledge-agent".equals(request.getTargetAgentId())) {
            return failed(request, "UNKNOWN_AGENT", "target is not on the allow list");
        }
        long timeout = effectiveTimeout(request);
        if (timeout <= 0) {
            return failed(request, "DEADLINE_EXCEEDED", "inbound deadline has already expired");
        }
        String inputJson = canonicalJson(request.getInput());
        String hash = sha256(inputJson);
        int leaseSeconds = (int) Math.max(45, timeout / 1000 + 15);
        InvocationStore.Claim claim = store.claim(caller, request.getInvocationId(), request.getTargetAgentId(), SKILL, hash, leaseSeconds);
        switch (claim.outcome()) {
            case "CONFLICT" -> {
                return failed(request, "IDEMPOTENCY_CONFLICT", "invocation_id was used with different input");
            }
            case "COMPLETED" -> {
                return completed(request, claim.resultJson());
            }
            case "RUN" -> {
                // 本次领取到了租约，继续执行。
            }
            default -> {
                return inProgress(request);
            }
        }
        try {
            String card = registry.cardUrl(request.getTargetAgentId());
            Map<String, Object> input = json.readValue(inputJson, new TypeReference<>() {});
            Duration budget = Duration.ofMillis(timeout);
            String result = governance.call(() -> knowledge.send(card, input, budget), budget);
            if (!store.finish(caller, request.getInvocationId(), claim.fencingToken(), "COMPLETED", result, null, null)) {
                return inProgress(request);
            }
            log.info("invocation_id={} traceparent={} status=COMPLETED timeout_ms={}", request.getInvocationId(), traceparent(), timeout);
            return completed(request, result);
        } catch (Exception error) {
            String status = timedOut(error) ? "TIMED_OUT" : unknown(error) ? "UNKNOWN" : "FAILED";
            String code = switch (status) {
                case "TIMED_OUT" -> "TIMED_OUT";
                case "UNKNOWN" -> "UNKNOWN";
                // 许可不足/熔断打开不是上游失败，给调用方可以区分的错误码。
                default -> error instanceof CallGovernance.BusyException ? "RESOURCE_EXHAUSTED"
                        : error instanceof CallGovernance.CircuitOpenException ? "UNAVAILABLE" : "UPSTREAM";
            };
            String message = error.getMessage() == null ? status : error.getMessage();
            store.finish(caller, request.getInvocationId(), claim.fencingToken(), status, null, code, message);
            log.info("invocation_id={} traceparent={} status={} code={} timeout_ms={}", request.getInvocationId(), traceparent(), status, code, timeout);
            return failed(request, code, message);
        }
    }

    // 预算 = min(入站剩余 Deadline, 请求 timeout_ms, 服务端上限)，不让每一跳重置为完整超时。
    private static long effectiveTimeout(InvokeAgentRequest request) {
        long requested = request.getTimeoutMs() > 0 ? request.getTimeoutMs() : MAX_TIMEOUT_MS;
        long timeout = Math.min(requested, MAX_TIMEOUT_MS);
        Deadline inbound = Context.current().getDeadline();
        if (inbound != null) {
            long remaining = inbound.timeRemaining(TimeUnit.MILLISECONDS);
            if (remaining <= 0) {
                return 0;
            }
            timeout = Math.min(timeout, remaining);
        }
        return timeout;
    }

    private static InvokeAgentResponse inProgress(InvokeAgentRequest request) {
        return InvokeAgentResponse.newBuilder()
                .setInvocationId(request.getInvocationId())
                .setStatus("IN_PROGRESS")
                .setSafeMessage("the same invocation is still running")
                .build();
    }

    private InvokeAgentResponse completed(InvokeAgentRequest request, String result) throws Exception {
        Struct.Builder struct = Struct.newBuilder();
        if (result != null && !result.isBlank()) {
            JsonFormat.parser().merge(result, struct);
        }
        return InvokeAgentResponse.newBuilder()
                .setInvocationId(request.getInvocationId())
                .setStatus("COMPLETED")
                .setResult(struct)
                .build();
    }

    private InvokeAgentResponse failed(InvokeAgentRequest request, String code, String message) {
        return InvokeAgentResponse.newBuilder()
                .setInvocationId(request.getInvocationId())
                .setStatus("FAILED")
                .setErrorCode(code == null ? "FAILED" : code)
                .setSafeMessage(message == null ? "" : message)
                .build();
    }

    private static String traceparent() {
        Metadata headers = HeaderForwarding.INCOMING.get();
        if (headers == null) {
            return "";
        }
        String value = headers.get(Metadata.Key.of("traceparent", Metadata.ASCII_STRING_MARSHALLER));
        return value == null ? "" : value;
    }

    private static String caller() {
        Metadata headers = HeaderForwarding.INCOMING.get();
        if (headers == null) {
            return "";
        }
        String value = headers.get(HeaderForwarding.CALLER);
        return value == null ? "" : value;
    }

    static String sha256(String value) throws Exception {
        byte[] digest = MessageDigest.getInstance("SHA-256").digest(value.getBytes(StandardCharsets.UTF_8));
        return HexFormat.of().formatHex(digest);
    }

    private static boolean timedOut(Throwable error) {
        String text = String.valueOf(error.getMessage()).toLowerCase();
        return text.contains("timeout") || text.contains("timed out") || text.contains("deadline");
    }

    private static boolean unknown(Throwable error) {
        String text = String.valueOf(error.getMessage()).toLowerCase();
        return text.contains("unknown") || text.contains("connection reset");
    }

    public static Map<String, Object> sorted(Map<String, Object> input) {
        return new TreeMap<>(input);
    }

    static String canonicalJson(com.google.protobuf.Struct input) throws Exception {
        String raw = JsonFormat.printer().omittingInsignificantWhitespace().print(input);
        Object parsed = new ObjectMapper().readValue(raw, Object.class);
        return new ObjectMapper().writeValueAsString(sortValue(parsed));
    }

    private static Object sortValue(Object value) {
        if (value instanceof Map<?, ?> map) {
            TreeMap<String, Object> sorted = new TreeMap<>();
            map.forEach((key, item) -> sorted.put(String.valueOf(key), sortValue(item)));
            return sorted;
        }
        if (value instanceof List<?> list) {
            List<Object> sorted = new ArrayList<>();
            for (Object item : list) {
                sorted.add(sortValue(item));
            }
            return sorted;
        }
        return value;
    }

    private static long env(String name, long fallback) {
        try {
            String raw = System.getenv(name);
            return raw == null || raw.isBlank() ? fallback : Long.parseLong(raw);
        } catch (NumberFormatException error) {
            return fallback;
        }
    }
}
