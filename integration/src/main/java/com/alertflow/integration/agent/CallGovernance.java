package com.alertflow.integration.agent;

import java.time.Duration;
import java.util.concurrent.Callable;
import java.util.concurrent.Semaphore;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicInteger;

import org.springframework.stereotype.Component;

// 只限制 Java 自己发出的远端调用，不参与 Episode 排队。
@Component
public class CallGovernance {
    // 许可不足是“资源忙”，与调用超时/上游失败分开暴露。
    public static class BusyException extends IllegalStateException {
        public BusyException(String message) {
            super(message);
        }
    }

    public static class CircuitOpenException extends IllegalStateException {
        public CircuitOpenException(String message) {
            super(message);
        }
    }

    private final Semaphore permits = new Semaphore(parse("INTEGRATION_MAX_INFLIGHT", 32));
    private final AtomicInteger failures = new AtomicInteger();
    private volatile long openUntil;

    public String call(Callable<String> action) throws Exception {
        return call(action, Duration.ofSeconds(2));
    }

    // 等待许可的时间计入调用方给出的剩余预算，而不是固定等待后再开始计时。
    public String call(Callable<String> action, Duration budget) throws Exception {
        if (System.currentTimeMillis() < openUntil) {
            throw new CircuitOpenException("circuit open");
        }
        long waitMillis = budget == null ? 2_000 : Math.max(1, budget.toMillis());
        if (!permits.tryAcquire(waitMillis, TimeUnit.MILLISECONDS)) {
            throw new BusyException("bulkhead full");
        }
        try {
            String value = action.call();
            failures.set(0);
            return value;
        } catch (Exception error) {
            if (failures.incrementAndGet() >= 5) {
                openUntil = System.currentTimeMillis() + 10_000;
            }
            throw error;
        } finally {
            permits.release();
        }
    }

    private static int parse(String name, int fallback) {
        try {
            String raw = System.getenv(name);
            return raw == null || raw.isBlank() ? fallback : Integer.parseInt(raw);
        } catch (NumberFormatException error) {
            return fallback;
        }
    }
}
