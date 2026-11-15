package com.alertflow.integration.mcp;

import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.time.Duration;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.Semaphore;
import java.util.concurrent.TimeUnit;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import org.springframework.stereotype.Component;

@Component
public class McpClientManager {
    public record Tool(String provider, String name, String description, String schemaJson) {}

    // Provider 拒绝服务时抛这个，网关映射为 JSON-RPC -32003，而不是“provider unavailable”。
    public static class ProviderBusyException extends RuntimeException {
        public ProviderBusyException(String message) {
            super(message);
        }
    }

    private final ObjectMapper json = new ObjectMapper();
    private final HttpClient http = HttpClient.newBuilder()
            .connectTimeout(Duration.ofMillis(env("INTEGRATION_MCP_CONNECT_TIMEOUT_MS", 2_000)))
            .build();
    private final Map<String, String> upstreams = new ConcurrentHashMap<>();
    private final Map<String, List<Tool>> catalog = new ConcurrentHashMap<>();
    private final Map<String, Semaphore> providerPermits = new ConcurrentHashMap<>();
    // 每个 Provider 的并发上限初始值来自本仓库当前 MCP Provider 的实测承载能力（单 Provider 无状态查询），
    // 不追求全局统一值；可按 Provider 单独通过环境变量在验证后调整。
    private final int providerMaxInFlight = (int) env("INTEGRATION_MCP_PROVIDER_MAX_INFLIGHT", 8);
    private final long callBudgetMillis = env("INTEGRATION_MCP_CALL_TIMEOUT_MS", 3_000);

    public McpClientManager() {
        String raw = System.getenv("INTEGRATION_MCP_UPSTREAMS");
        if (raw != null) {
            for (String item : raw.split(",")) {
                int eq = item.indexOf('=');
                if (eq > 0) {
                    add(item.substring(0, eq).trim(), item.substring(eq + 1).trim());
                }
            }
        }
    }

    public void remember(Tool tool) {
        catalog.computeIfAbsent(tool.provider(), key -> new ArrayList<>()).add(tool);
        upstreams.putIfAbsent(tool.provider(), "memory");
    }

    public void add(String provider, String endpoint) {
        upstreams.put(provider, endpoint);
        refresh(provider);
    }

    public List<Tool> tools() {
        List<Tool> all = new ArrayList<>();
        catalog.values().forEach(all::addAll);
        return all;
    }

    public String call(String provider, String tool, String arguments) throws Exception {
        String endpoint = upstreams.get(provider);
        if (endpoint == null) {
            throw new IllegalStateException("provider unavailable");
        }
        Semaphore permits = providerPermits.computeIfAbsent(provider, key -> new Semaphore(providerMaxInFlight));
        long deadline = System.nanoTime() + TimeUnit.MILLISECONDS.toNanos(callBudgetMillis);
        long waitMillis = Math.max(1, TimeUnit.NANOSECONDS.toMillis(deadline - System.nanoTime()));
        if (!permits.tryAcquire(waitMillis, TimeUnit.MILLISECONDS)) {
            throw new ProviderBusyException("provider in-flight limit reached");
        }
        try {
            long remaining = Math.max(1, TimeUnit.NANOSECONDS.toMillis(deadline - System.nanoTime()));
            String body = "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/call\",\"params\":{\"name\":\""
                    + tool + "\",\"arguments\":" + arguments + "}}";
            JsonNode response = post(endpoint, body, remaining);
            if (response.has("error")) {
                throw new IllegalStateException("provider unavailable");
            }
            return response.path("result").path("content").path(0).path("text").asText();
        } finally {
            permits.release();
        }
    }

    private void refresh(String provider) {
        try {
            JsonNode response = post(upstreams.get(provider), "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/list\"}", callBudgetMillis);
            List<Tool> tools = new ArrayList<>();
            for (JsonNode tool : response.path("result").path("tools")) {
                tools.add(new Tool(provider, tool.path("name").asText(), tool.path("description").asText(""), tool.path("inputSchema").toString()));
            }
            catalog.put(provider, tools);
        } catch (Exception error) {
            catalog.remove(provider);
        }
    }

    private JsonNode post(String endpoint, String body, long timeoutMillis) throws Exception {
        HttpRequest request = HttpRequest.newBuilder(URI.create(endpoint))
                .timeout(Duration.ofMillis(Math.max(1, timeoutMillis)))
                .header("content-type", "application/json")
                .POST(HttpRequest.BodyPublishers.ofString(body))
                .build();
        HttpResponse<String> response = http.send(request, HttpResponse.BodyHandlers.ofString());
        return json.readTree(response.body());
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
