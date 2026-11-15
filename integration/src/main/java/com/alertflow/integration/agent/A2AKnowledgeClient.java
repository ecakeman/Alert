package com.alertflow.integration.agent;

import java.net.http.HttpClient;
import java.time.Duration;
import java.util.Map;
import java.util.UUID;
import java.util.concurrent.ExecutionException;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.Future;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.TimeoutException;
import java.util.concurrent.atomic.AtomicReference;

import com.fasterxml.jackson.databind.ObjectMapper;
import jakarta.annotation.PreDestroy;
import org.a2aproject.sdk.A2A;
import org.a2aproject.sdk.client.Client;
import org.a2aproject.sdk.client.MessageEvent;
import org.a2aproject.sdk.client.config.ClientConfig;
import org.a2aproject.sdk.client.http.JdkA2AHttpClient;
import org.a2aproject.sdk.client.transport.jsonrpc.JSONRPCTransport;
import org.a2aproject.sdk.client.transport.jsonrpc.JSONRPCTransportConfigBuilder;
import org.a2aproject.sdk.client.transport.spi.interceptors.ClientCallContext;
import org.a2aproject.sdk.spec.AgentCard;
import org.a2aproject.sdk.spec.AgentSkill;
import org.a2aproject.sdk.spec.DataPart;
import org.a2aproject.sdk.spec.Message;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.stereotype.Component;

// A2A 同步调用：应用级共享虚拟线程执行器 + 共享 HTTP 客户端。
// 单次调用仍以 Future.get(timeout) 作整体边界；超时/中断时 cancel 本地任务，不承诺远端回滚。
@Component
public class A2AKnowledgeClient implements KnowledgeClient, AutoCloseable {
    private final ObjectMapper json = new ObjectMapper();
    private final String token;
    private final ExecutorService executor = Executors.newVirtualThreadPerTaskExecutor();
    private final HttpClient http;
    private final JdkA2AHttpClient sharedHttp;
    private final long cardTimeoutMillis = env("INTEGRATION_A2A_CARD_TIMEOUT_MS", 5_000);

    @Autowired
    public A2AKnowledgeClient() {
        this(System.getenv("KNOWLEDGE_A2A_TOKEN"));
    }

    public A2AKnowledgeClient(String token) {
        this.token = token == null ? "" : token;
        this.http = HttpClient.newBuilder()
                .version(HttpClient.Version.HTTP_1_1)
                .connectTimeout(Duration.ofMillis(env("INTEGRATION_A2A_CONNECT_TIMEOUT_MS", 2_000)))
                .build();
        this.sharedHttp = new JdkA2AHttpClient(http);
    }

    @Override
    public String send(String cardUrl, Map<String, Object> input, Duration timeout) throws Exception {
        Duration budget = timeout == null || timeout.isNegative() || timeout.isZero()
                ? Duration.ofSeconds(30) : timeout;
        long deadline = System.nanoTime() + budget.toNanos();
        Future<String> future = executor.submit(() -> sendWithinCaller(cardUrl, input, deadline));
        try {
            return future.get(budget.toMillis(), TimeUnit.MILLISECONDS);
        } catch (TimeoutException error) {
            future.cancel(true);
            throw new IllegalStateException("timed out", error);
        } catch (InterruptedException error) {
            future.cancel(true);
            Thread.currentThread().interrupt();
            throw new IllegalStateException("timed out", error);
        } catch (ExecutionException error) {
            Throwable cause = error.getCause() == null ? error : error.getCause();
            throw new IllegalStateException(cause.getMessage(), cause);
        }
    }

    private String sendWithinCaller(String cardUrl, Map<String, Object> input, long deadline) throws Exception {
        AgentCard card = fetchCard(cardUrl, deadline);
        boolean skill = false;
        for (AgentSkill item : card.skills()) {
            if ("knowledge-retrieval".equals(item.id())) {
                skill = true;
            }
        }
        if (!skill) {
            throw new IllegalStateException("agent card has no knowledge-retrieval skill");
        }
        if (card.capabilities().streaming() || card.capabilities().pushNotifications()) {
            throw new IllegalStateException("knowledge agent must not advertise streaming");
        }
        Duration remaining = remaining(deadline);
        AtomicReference<Object> data = new AtomicReference<>();
        AtomicReference<Throwable> failure = new AtomicReference<>();
        ClientConfig config = new ClientConfig.Builder().setStreaming(false).build();
        Message message = Message.builder()
                .role(Message.Role.ROLE_USER)
                .messageId(UUID.randomUUID().toString())
                .parts(new DataPart(input))
                .build();
        try (Client client = Client.builder(card)
                .clientConfig(config)
                .withTransport(JSONRPCTransport.class, new JSONRPCTransportConfigBuilder().httpClient(sharedHttp))
                .addConsumer((event, ignored) -> {
                    if (event instanceof MessageEvent messageEvent) {
                        messageEvent.getMessage().parts().forEach(part -> {
                            if (part instanceof DataPart dataPart) {
                                data.set(dataPart.data());
                            }
                        });
                    }
                })
                .streamingErrorHandler(failure::set)
                .build()) {
            client.sendMessage(message, callContext(remaining));
        }
        if (failure.get() != null) {
            throw new IllegalStateException(failure.get().getMessage(), failure.get());
        }
        if (data.get() == null) {
            throw new IllegalStateException("unknown: A2A response had no data part");
        }
        return json.writeValueAsString(data.get());
    }

    // Agent Card 获取也必须在整体预算内，并额外受独立上限约束（防止上游不回包时线程长时间挂起）。
    private AgentCard fetchCard(String cardUrl, long deadline) throws Exception {
        long remainingMillis = TimeUnit.NANOSECONDS.toMillis(deadline - System.nanoTime());
        if (remainingMillis <= 0) {
            throw new IllegalStateException("agent card fetch timed out");
        }
        long budgetMillis = Math.min(remainingMillis, cardTimeoutMillis);
        Future<AgentCard> future = executor.submit(() -> A2A.getAgentCard(sharedHttp, cardUrl));
        try {
            return future.get(budgetMillis, TimeUnit.MILLISECONDS);
        } catch (TimeoutException error) {
            future.cancel(true);
            throw new IllegalStateException("agent card fetch timed out", error);
        } catch (InterruptedException error) {
            future.cancel(true);
            Thread.currentThread().interrupt();
            throw new IllegalStateException("agent card fetch interrupted", error);
        } catch (ExecutionException error) {
            Throwable cause = error.getCause() == null ? error : error.getCause();
            throw new IllegalStateException(cause.getMessage(), cause);
        }
    }

    private ClientCallContext callContext(Duration timeout) {
        if (token.isBlank()) {
            throw new IllegalStateException("KNOWLEDGE_A2A_TOKEN is required");
        }
        long millis = Math.max(1, timeout.toMillis());
        return new ClientCallContext(Map.of(), Map.of(
                "Authorization", "Bearer " + token,
                "X-Knowledge-Timeout-Ms", Long.toString(millis)));
    }

    private static Duration remaining(long deadline) {
        long millis = Math.max(1, TimeUnit.NANOSECONDS.toMillis(deadline - System.nanoTime()));
        return Duration.ofMillis(millis);
    }

    private static long env(String name, long fallback) {
        try {
            String raw = System.getenv(name);
            return raw == null || raw.isBlank() ? fallback : Long.parseLong(raw);
        } catch (NumberFormatException error) {
            return fallback;
        }
    }

    @PreDestroy
    @Override
    public void close() {
        executor.shutdownNow();
    }
}
