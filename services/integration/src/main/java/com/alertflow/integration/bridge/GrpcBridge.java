package com.alertflow.integration.bridge;

import java.util.concurrent.Callable;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.Semaphore;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicBoolean;

import com.alertflow.gen.agent.v1.AgentExecutionServiceGrpc;
import com.alertflow.gen.agent.v1.CompleteTaskRequest;
import com.alertflow.gen.agent.v1.GetArtifactRequest;
import com.alertflow.gen.agent.v1.GetArtifactResponse;
import com.alertflow.gen.agent.v1.GetEpisodeInputRequest;
import com.alertflow.gen.agent.v1.GetEpisodeInputResponse;
import com.alertflow.gen.agent.v1.HeartbeatTaskRequest;
import com.alertflow.gen.agent.v1.QueryTimelineRequest;
import com.alertflow.gen.agent.v1.QueryTimelineResponse;
import com.alertflow.gen.agent.v1.RegisterArtifactRequest;
import com.alertflow.gen.agent.v1.RegisterArtifactResponse;
import com.alertflow.gen.agent.v1.RegisterFaultDocumentRequest;
import com.alertflow.gen.agent.v1.RegisterFaultDocumentResponse;
import com.alertflow.gen.agent.v1.ReportTaskStartedRequest;
import com.alertflow.gen.agent.v1.SubmitTaskRequest;
import com.alertflow.gen.agent.v1.SubmitTaskResponse;
import com.alertflow.gen.agent.v1.SystemGatewayServiceGrpc;
import com.alertflow.gen.agent.v1.TaskAck;
import com.alertflow.gen.integration.v1.AgentInvocationServiceGrpc;
import com.alertflow.gen.system.v1.SystemApiGrpc;
import com.alertflow.gen.integration.v1.InvokeAgentRequest;
import com.alertflow.gen.integration.v1.InvokeAgentResponse;
import com.alertflow.integration.agent.InvocationService;
import io.grpc.Context;
import io.grpc.Deadline;
import io.grpc.Grpc;
import io.grpc.InsecureChannelCredentials;
import io.grpc.InsecureServerCredentials;
import io.grpc.ManagedChannel;
import io.grpc.Server;
import io.grpc.Status;
import io.grpc.StatusRuntimeException;
import io.grpc.stub.StreamObserver;
import jakarta.annotation.PreDestroy;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.stereotype.Component;

// Go 和 Python 之间的两个既有服务，以及新的 InvokeAgent。Java 不改业务字段。
// 同步转发跑在 gRPC 自己的虚拟线程执行器上；阻塞调用不占用平台线程。
@Component
public class GrpcBridge implements AutoCloseable {
    private static final Logger log = LoggerFactory.getLogger(GrpcBridge.class);
    private final AtomicBoolean closed = new AtomicBoolean();
    private final Semaphore pythonPermits = new Semaphore((int) env("INTEGRATION_PYTHON_MAX_INFLIGHT", 8));
    private final Semaphore goPermits = new Semaphore((int) env("INTEGRATION_GO_MAX_INFLIGHT", 16));
    private final long pythonDefaultMillis = env("INTEGRATION_PYTHON_CALL_TIMEOUT_MS", 10_000);
    private final long goDefaultMillis = env("INTEGRATION_GO_CALL_TIMEOUT_MS", 10_000);
    private final long shutdownGraceMillis = env("INTEGRATION_GRPC_SHUTDOWN_TIMEOUT_MS", 15_000);

    private Server server;
    private ManagedChannel python;
    private ManagedChannel go;
    private ExecutorService grpcExecutor;
    private int port;

    @Autowired
    public GrpcBridge(InvocationService invocations) {
        try {
            start(env("INTEGRATION_PYTHON_GRPC_TARGET", "127.0.0.1:9000"),
                    env("INTEGRATION_GO_GRPC_TARGET", "127.0.0.1:9090"),
                    port(env("INTEGRATION_GRPC_ADDR", ":9091")),
                    invocations);
        } catch (Exception error) {
            throw new IllegalStateException(error);
        }
    }

    public GrpcBridge(String pythonTarget, String goTarget, int port, InvocationService invocations) throws Exception {
        start(pythonTarget, goTarget, port, invocations);
    }

    private void start(String pythonTarget, String goTarget, int port, InvocationService invocations) throws Exception {
        this.grpcExecutor = Executors.newVirtualThreadPerTaskExecutor();
        this.python = Grpc.newChannelBuilder(pythonTarget, InsecureChannelCredentials.create()).build();
        this.go = Grpc.newChannelBuilder(goTarget, InsecureChannelCredentials.create()).build();
        AgentExecutionServiceGrpc.AgentExecutionServiceBlockingStub pythonStub =
                AgentExecutionServiceGrpc.newBlockingStub(python).withInterceptors(new HeaderForwarding());
        SystemGatewayServiceGrpc.SystemGatewayServiceBlockingStub goStub =
                SystemGatewayServiceGrpc.newBlockingStub(go).withInterceptors(new HeaderForwarding());
        // 每个 RPC 一条虚拟线程，且由 gRPC 自己管理；不使用 directExecutor。
        this.server = Grpc.newServerBuilderForPort(port, InsecureServerCredentials.create())
                .executor(grpcExecutor)
                .intercept(new HeaderForwarding())
                .addService(new ExecutionProxy(pythonStub, pythonPermits, pythonDefaultMillis))
                .addService(new GatewayProxy(goStub, goPermits, goDefaultMillis))
                .addService(new InvocationProxy(invocations))
                .build()
                .start();
        this.port = server.getPort();
    }

    @PreDestroy
    public void close() {
        if (!closed.compareAndSet(false, true)) {
            return;
        }
        try {
            // 1) 先停止接收新 RPC，并给在途请求留排空时间。
            server.shutdown();
            if (!server.awaitTermination(shutdownGraceMillis, TimeUnit.MILLISECONDS)) {
                log.warn("gRPC server did not drain in {}ms, forcing shutdown", shutdownGraceMillis);
                server.shutdownNow();
                server.awaitTermination(2, TimeUnit.SECONDS);
            }
            // 2) 再关闭出站 Channel。
            shutdownChannel(python);
            shutdownChannel(go);
        } catch (InterruptedException interrupted) {
            server.shutdownNow();
            python.shutdownNow();
            go.shutdownNow();
            Thread.currentThread().interrupt();
        } finally {
            // 3) 最后关闭 gRPC 执行器；它不承载 A2A/HTTP 任务。
            grpcExecutor.shutdown();
            try {
                if (!grpcExecutor.awaitTermination(2, TimeUnit.SECONDS)) {
                    grpcExecutor.shutdownNow();
                }
            } catch (InterruptedException interrupted) {
                grpcExecutor.shutdownNow();
                Thread.currentThread().interrupt();
            }
        }
    }

    private static void shutdownChannel(ManagedChannel channel) throws InterruptedException {
        channel.shutdown();
        if (!channel.awaitTermination(2, TimeUnit.SECONDS)) {
            channel.shutdownNow();
        }
    }

    public int port() {
        return port;
    }

    public SystemApiGrpc.SystemApiBlockingStub systemApi() {
        Deadline budget = callBudget(goDefaultMillis);
        SystemApiGrpc.SystemApiBlockingStub stub = SystemApiGrpc.newBlockingStub(go);
        return Context.current().getDeadline() != null ? stub : stub.withDeadline(budget);
    }

    public AgentExecutionServiceGrpc.AgentExecutionServiceBlockingStub pythonExecution() {
        Deadline budget = callBudget(pythonDefaultMillis);
        AgentExecutionServiceGrpc.AgentExecutionServiceBlockingStub stub =
                AgentExecutionServiceGrpc.newBlockingStub(python);
        return Context.current().getDeadline() != null ? stub : stub.withDeadline(budget);
    }

    public <T> T callPython(Callable<T> call) throws Exception {
        Deadline budget = callBudget(pythonDefaultMillis);
        long waitMillis = Math.max(1, budget.timeRemaining(TimeUnit.MILLISECONDS));
        if (!pythonPermits.tryAcquire(waitMillis, TimeUnit.MILLISECONDS)) {
            throw Status.RESOURCE_EXHAUSTED.withDescription("in-flight limit reached").asRuntimeException();
        }
        try {
            return call.call();
        } finally {
            pythonPermits.release();
        }
    }

    public <T> T callGo(Callable<T> call) throws Exception {
        Deadline budget = callBudget(goDefaultMillis);
        long waitMillis = Math.max(1, budget.timeRemaining(TimeUnit.MILLISECONDS));
        if (!goPermits.tryAcquire(waitMillis, TimeUnit.MILLISECONDS)) {
            throw Status.RESOURCE_EXHAUSTED.withDescription("in-flight limit reached").asRuntimeException();
        }
        try {
            return call.call();
        } finally {
            goPermits.release();
        }
    }

    // 在途许可等待与 RPC 本身共享同一份预算，避免“排队时间不计入超时”。
    private static Deadline callBudget(long defaultMillis) {
        Deadline inbound = Context.current().getDeadline();
        return inbound != null ? inbound : Deadline.after(defaultMillis, TimeUnit.MILLISECONDS);
    }

    private static <T> void forward(StreamObserver<T> observer, Semaphore permits, Deadline budget, Callable<T> call) {
        long waitMillis = Math.max(1, budget.timeRemaining(TimeUnit.MILLISECONDS));
        boolean acquired = false;
        try {
            acquired = permits.tryAcquire(waitMillis, TimeUnit.MILLISECONDS);
        } catch (InterruptedException interrupted) {
            Thread.currentThread().interrupt();
            observer.onError(Status.CANCELLED.withDescription("interrupted while waiting for an in-flight permit").asRuntimeException());
            return;
        }
        if (!acquired) {
            observer.onError(Status.RESOURCE_EXHAUSTED.withDescription("in-flight limit reached").asRuntimeException());
            return;
        }
        try {
            finish(observer, call.call());
        } catch (Exception error) {
            fail(observer, error);
        } finally {
            permits.release();
        }
    }

    private static String env(String key, String fallback) {
        String value = System.getenv(key);
        return value == null || value.isBlank() ? fallback : value;
    }

    private static long env(String key, long fallback) {
        try {
            return Long.parseLong(env(key, Long.toString(fallback)));
        } catch (NumberFormatException error) {
            return fallback;
        }
    }

    private static int port(String addr) {
        int colon = addr.lastIndexOf(':');
        return Integer.parseInt(colon < 0 ? addr : addr.substring(colon + 1));
    }

    private static <T> void finish(StreamObserver<T> observer, T value) {
        observer.onNext(value);
        observer.onCompleted();
    }

    private static void fail(StreamObserver<?> observer, Exception error) {
        if (error instanceof StatusRuntimeException status) {
            observer.onError(status);
            return;
        }
        observer.onError(Status.INTERNAL.withDescription(error.getMessage()).asRuntimeException());
    }

    static final class ExecutionProxy extends AgentExecutionServiceGrpc.AgentExecutionServiceImplBase {
        private final AgentExecutionServiceGrpc.AgentExecutionServiceBlockingStub python;
        private final Semaphore permits;
        private final long defaultMillis;

        ExecutionProxy(AgentExecutionServiceGrpc.AgentExecutionServiceBlockingStub python, Semaphore permits, long defaultMillis) {
            this.python = python;
            this.permits = permits;
            this.defaultMillis = defaultMillis;
        }

        @Override
        public void submitTask(SubmitTaskRequest request, StreamObserver<SubmitTaskResponse> observer) {
            Deadline budget = callBudget(defaultMillis);
            var stub = Context.current().getDeadline() != null ? python : python.withDeadline(budget);
            forward(observer, permits, budget, () -> stub.submitTask(request));
        }

        @Override
        public void getArtifact(GetArtifactRequest request, StreamObserver<GetArtifactResponse> observer) {
            Deadline budget = callBudget(defaultMillis);
            var stub = Context.current().getDeadline() != null ? python : python.withDeadline(budget);
            forward(observer, permits, budget, () -> stub.getArtifact(request));
        }
    }

    static final class GatewayProxy extends SystemGatewayServiceGrpc.SystemGatewayServiceImplBase {
        private final SystemGatewayServiceGrpc.SystemGatewayServiceBlockingStub go;
        private final Semaphore permits;
        private final long defaultMillis;

        GatewayProxy(SystemGatewayServiceGrpc.SystemGatewayServiceBlockingStub go, Semaphore permits, long defaultMillis) {
            this.go = go;
            this.permits = permits;
            this.defaultMillis = defaultMillis;
        }

        @Override
        public void getEpisodeInput(GetEpisodeInputRequest request, StreamObserver<GetEpisodeInputResponse> observer) {
            Deadline budget = callBudget(defaultMillis);
            var stub = Context.current().getDeadline() != null ? go : go.withDeadline(budget);
            forward(observer, permits, budget, () -> stub.getEpisodeInput(request));
        }

        @Override
        public void queryTimeline(QueryTimelineRequest request, StreamObserver<QueryTimelineResponse> observer) {
            Deadline budget = callBudget(defaultMillis);
            var stub = Context.current().getDeadline() != null ? go : go.withDeadline(budget);
            forward(observer, permits, budget, () -> stub.queryTimeline(request));
        }

        @Override
        public void registerArtifact(RegisterArtifactRequest request, StreamObserver<RegisterArtifactResponse> observer) {
            Deadline budget = callBudget(defaultMillis);
            var stub = Context.current().getDeadline() != null ? go : go.withDeadline(budget);
            forward(observer, permits, budget, () -> stub.registerArtifact(request));
        }

        @Override
        public void reportTaskStarted(ReportTaskStartedRequest request, StreamObserver<TaskAck> observer) {
            Deadline budget = callBudget(defaultMillis);
            var stub = Context.current().getDeadline() != null ? go : go.withDeadline(budget);
            forward(observer, permits, budget, () -> stub.reportTaskStarted(request));
        }

        @Override
        public void heartbeatTask(HeartbeatTaskRequest request, StreamObserver<TaskAck> observer) {
            Deadline budget = callBudget(defaultMillis);
            var stub = Context.current().getDeadline() != null ? go : go.withDeadline(budget);
            forward(observer, permits, budget, () -> stub.heartbeatTask(request));
        }

        @Override
        public void completeTask(CompleteTaskRequest request, StreamObserver<TaskAck> observer) {
            Deadline budget = callBudget(defaultMillis);
            var stub = Context.current().getDeadline() != null ? go : go.withDeadline(budget);
            forward(observer, permits, budget, () -> stub.completeTask(request));
        }

        @Override
        public void registerFaultDocument(RegisterFaultDocumentRequest request, StreamObserver<RegisterFaultDocumentResponse> observer) {
            Deadline budget = callBudget(defaultMillis);
            var stub = Context.current().getDeadline() != null ? go : go.withDeadline(budget);
            forward(observer, permits, budget, () -> stub.registerFaultDocument(request));
        }
    }

    static final class InvocationProxy extends AgentInvocationServiceGrpc.AgentInvocationServiceImplBase {
        private final InvocationService invocations;

        InvocationProxy(InvocationService invocations) {
            this.invocations = invocations;
        }

        @Override
        public void invokeAgent(InvokeAgentRequest request, StreamObserver<InvokeAgentResponse> observer) {
            try {
                finish(observer, invocations.invoke(request));
            } catch (Exception error) {
                fail(observer, error);
            }
        }
    }
}
