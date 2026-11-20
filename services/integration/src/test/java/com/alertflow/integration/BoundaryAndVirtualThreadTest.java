package com.alertflow.integration;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import java.time.Duration;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicReference;

import com.alertflow.gen.system.v1.JsonResult;
import com.alertflow.gen.system.v1.SystemApiGrpc;
import com.alertflow.gen.agent.v1.AgentExecutionServiceGrpc;
import com.alertflow.integration.api.PublicApiController;
import com.alertflow.integration.cache.RevisionCache;
import com.alertflow.gen.agent.v1.SubmitTaskRequest;
import com.alertflow.gen.agent.v1.SubmitTaskResponse;
import com.alertflow.integration.agent.A2AKnowledgeClient;
import com.alertflow.integration.agent.AgentRegistry;
import com.alertflow.integration.agent.CallGovernance;
import com.alertflow.integration.agent.InvocationService;
import com.alertflow.integration.agent.MemoryInvocationStore;
import com.alertflow.integration.bridge.GrpcBridge;
import com.alertflow.integration.mcp.McpClientManager;
import com.alertflow.integration.mcp.McpController;
import com.alertflow.integration.mcp.McpGateway;
import com.alertflow.integration.mcp.McpInfrastructureException;
import com.alertflow.integration.mcp.McpPolicy;
import io.grpc.Context;
import io.grpc.Deadline;
import io.grpc.Grpc;
import io.grpc.InsecureChannelCredentials;
import io.grpc.InsecureServerCredentials;
import io.grpc.Server;
import io.grpc.stub.StreamObserver;
import org.junit.jupiter.api.Test;
import org.springframework.http.ResponseEntity;

import jakarta.servlet.http.HttpServletRequest;

import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.times;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

// A2A/HTTP/gRPC 边界的超时与错误语义测试。
class BoundaryAndVirtualThreadTest {
    @Test
    void querySensitiveListsKeepTheirOwnQueryThroughCache() throws Exception {
        GrpcBridge bridge = mock(GrpcBridge.class);
        RevisionCache cache = mock(RevisionCache.class);
        SystemApiGrpc.SystemApiBlockingStub api = mock(SystemApiGrpc.SystemApiBlockingStub.class);
        when(bridge.systemApi()).thenReturn(api);
        when(cache.around(org.mockito.ArgumentMatchers.anyString(), org.mockito.ArgumentMatchers.anyString(),
                org.mockito.ArgumentMatchers.any(), org.mockito.ArgumentMatchers.any())).thenAnswer(invocation ->
                ((RevisionCache.Loader) invocation.getArgument(3)).load());
        when(api.listServices(org.mockito.ArgumentMatchers.any())).thenAnswer(invocation ->
                JsonResult.newBuilder().setHttpStatus(200).setBody("services:" + invocation.getArgument(0, com.alertflow.gen.system.v1.JsonCall.class).getQuery()).build());
        when(api.listEpisodes(org.mockito.ArgumentMatchers.any())).thenAnswer(invocation ->
                JsonResult.newBuilder().setHttpStatus(200).setBody("episodes:" + invocation.getArgument(0, com.alertflow.gen.system.v1.JsonCall.class).getQuery()).build());
        PublicApiController controller = new PublicApiController(bridge, cache);

        HttpServletRequest servicesActive = mock(HttpServletRequest.class);
        when(servicesActive.getQueryString()).thenReturn("status=UNKNOWN");
        HttpServletRequest servicesAbnormal = mock(HttpServletRequest.class);
        when(servicesAbnormal.getQueryString()).thenReturn("status=ABNORMAL");
        HttpServletRequest episodesPageOne = mock(HttpServletRequest.class);
        when(episodesPageOne.getQueryString()).thenReturn("status=ACTIVE&limit=1&offset=0");
        HttpServletRequest episodesPageTwo = mock(HttpServletRequest.class);
        when(episodesPageTwo.getQueryString()).thenReturn("status=ACTIVE&limit=50&offset=0");

        assertEquals("services:status=UNKNOWN", controller.listServices(servicesActive).getBody());
        assertEquals("services:status=ABNORMAL", controller.listServices(servicesAbnormal).getBody());
        assertEquals("episodes:status=ACTIVE&limit=1&offset=0", controller.listEpisodes(episodesPageOne).getBody());
        assertEquals("episodes:status=ACTIVE&limit=50&offset=0", controller.listEpisodes(episodesPageTwo).getBody());

        verify(api, times(2)).listServices(org.mockito.ArgumentMatchers.any());
        verify(api, times(2)).listEpisodes(org.mockito.ArgumentMatchers.any());
        verify(cache, times(4)).around(org.mockito.ArgumentMatchers.anyString(), org.mockito.ArgumentMatchers.anyString(),
                org.mockito.ArgumentMatchers.any(), org.mockito.ArgumentMatchers.any());
    }

    @Test
    void mcpControllerMapsAuthBadRequestAndInfrastructure() {
        McpPolicy policy = McpPolicy.memory();
        policy.remember("main-token", "alertflow-main-agent");
        policy.grant("alertflow-main-agent", "mock", "echo");
        McpClientManager clients = new McpClientManager();
        clients.remember(new McpClientManager.Tool("mock", "echo", "echo", "{\"type\":\"object\"}"));
        McpController controller = new McpController(new McpGateway(policy, clients));

        ResponseEntity<String> ok = controller.mcp("Bearer main-token",
                "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/list\"}");
        assertEquals(200, ok.getStatusCode().value());
        assertTrue(ok.getBody() != null && ok.getBody().contains("mock.echo"));

        ResponseEntity<String> unauthorized = controller.mcp(null,
                "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/list\"}");
        assertEquals(401, unauthorized.getStatusCode().value());

        ResponseEntity<String> invalid = controller.mcp("Bearer main-token", "{not-json");
        assertEquals(400, invalid.getStatusCode().value());

        McpPolicy broken = new McpPolicy(null) {
            @Override
            public String agentFor(String authorization) {
                throw new McpInfrastructureException("db down", new IllegalStateException());
            }
        };
        McpController failing = new McpController(new McpGateway(broken, clients));
        ResponseEntity<String> serverError = failing.mcp("Bearer main-token",
                "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/list\"}");
        assertEquals(500, serverError.getStatusCode().value());
    }

    @Test
    void a2aSendIsBoundedByTimeoutAndCancelsLocally() throws Exception {
        com.sun.net.httpserver.HttpServer slow = com.sun.net.httpserver.HttpServer.create(new java.net.InetSocketAddress("127.0.0.1", 0), 0);
        slow.createContext("/", exchange -> {
            try {
                Thread.sleep(3_000);
            } catch (InterruptedException ignored) {
                Thread.currentThread().interrupt();
            }
            exchange.close();
        });
        slow.start();
        try (A2AKnowledgeClient client = new A2AKnowledgeClient("test-token")) {
            long started = System.nanoTime();
            IllegalStateException error = assertThrows(IllegalStateException.class,
                    () -> client.send("http://127.0.0.1:" + slow.getAddress().getPort(), java.util.Map.of(), Duration.ofMillis(400)));
            long elapsedMillis = TimeUnit.NANOSECONDS.toMillis(System.nanoTime() - started);
            assertTrue(error.getMessage() == null || error.getMessage().contains("timed out"), "unexpected message: " + error.getMessage());
            assertTrue(elapsedMillis < 2_500, "send did not return promptly: " + elapsedMillis + "ms");
        } finally {
            slow.stop(0);
        }
    }

    @Test
    void grpcBridgePropagatesInboundDeadlineAndAppliesDefaultWhenMissing() throws Exception {
        AtomicReference<Deadline> seen = new AtomicReference<>();
        Server python = Grpc.newServerBuilderForPort(0, InsecureServerCredentials.create())
                .addService(new AgentExecutionServiceGrpc.AgentExecutionServiceImplBase() {
                    @Override
                    public void submitTask(SubmitTaskRequest request, StreamObserver<SubmitTaskResponse> observer) {
                        seen.set(Context.current().getDeadline());
                        observer.onNext(SubmitTaskResponse.newBuilder().setAccepted(true).setTaskId(request.getTaskId()).build());
                        observer.onCompleted();
                    }
                }).build().start();
        Server go = Grpc.newServerBuilderForPort(0, InsecureServerCredentials.create()).build().start();
        InvocationService unused = new InvocationService(new MemoryInvocationStore(), new AgentRegistry(), (card, input, timeout) -> "{}", new CallGovernance());
        try (GrpcBridge bridge = new GrpcBridge("127.0.0.1:" + python.getPort(), "127.0.0.1:" + go.getPort(), 0, unused)) {
            var channel = Grpc.newChannelBuilder("127.0.0.1:" + bridge.port(), InsecureChannelCredentials.create()).build();
            try {
                AgentExecutionServiceGrpc.newBlockingStub(channel)
                        .withDeadlineAfter(1_500, TimeUnit.MILLISECONDS)
                        .submitTask(SubmitTaskRequest.newBuilder().setTaskId("deadline-1").build());
                assertNotNull(seen.get(), "inbound deadline was not propagated");
                assertTrue(seen.get().timeRemaining(TimeUnit.MILLISECONDS) <= 1_500, "deadline was extended");

                seen.set(null);
                AgentExecutionServiceGrpc.newBlockingStub(channel)
                        .submitTask(SubmitTaskRequest.newBuilder().setTaskId("deadline-2").build());
                assertNotNull(seen.get(), "default deadline was not applied");
                long remaining = seen.get().timeRemaining(TimeUnit.MILLISECONDS);
                assertTrue(remaining > 0 && remaining <= 10_000, "default deadline out of range: " + remaining);
            } finally {
                channel.shutdownNow();
            }
        } finally {
            python.shutdownNow().awaitTermination(2, TimeUnit.SECONDS);
            go.shutdownNow().awaitTermination(2, TimeUnit.SECONDS);
        }
    }

    @Test
    void grpcBridgeCloseIsIdempotent() throws Exception {
        Server python = Grpc.newServerBuilderForPort(0, InsecureServerCredentials.create()).build().start();
        Server go = Grpc.newServerBuilderForPort(0, InsecureServerCredentials.create()).build().start();
        InvocationService unused = new InvocationService(new MemoryInvocationStore(), new AgentRegistry(), (card, input, timeout) -> "{}", new CallGovernance());
        GrpcBridge bridge = new GrpcBridge("127.0.0.1:" + python.getPort(), "127.0.0.1:" + go.getPort(), 0, unused);
        try {
            bridge.close();
            bridge.close();
        } finally {
            python.shutdownNow().awaitTermination(2, TimeUnit.SECONDS);
            go.shutdownNow().awaitTermination(2, TimeUnit.SECONDS);
        }
    }

}
