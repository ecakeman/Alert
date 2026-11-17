package monitoring

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/ecakeman/alertflow/services/system-core/internal/monitoring/probe/http"
	"github.com/ecakeman/alertflow/services/system-core/internal/monitoring/probe/kubernetes"
	"github.com/ecakeman/alertflow/services/system-core/internal/monitoring/probe/tcp"
	"github.com/ecakeman/alertflow/services/system-core/internal/monitoring/queue"
	"github.com/ecakeman/alertflow/services/system-core/internal/monitoring/scheduler"
	"github.com/ecakeman/alertflow/services/system-core/internal/monitoring/worker"
	"github.com/ecakeman/alertflow/services/system-core/internal/system"
	"github.com/google/uuid"
)

// Store 是调度器需要的系统读写面。
type Store interface {
	ListDueTargets(ctx context.Context) ([]system.DueTarget, error)
	ApplyMonitorSample(ctx context.Context, sample system.MonitorSample) (system.ApplyResult, error)
}

// Config 是监控引擎的容量。
type Config struct {
	Workers   int
	QueueSize int
	Timeout   time.Duration
	Reconcile time.Duration
	Tick      time.Duration
}

func ConfigFromEnv() Config {
	workers := envInt("ALERTFLOW_MONITOR_WORKERS", 4)
	return Config{
		Workers:   workers,
		QueueSize: envInt("ALERTFLOW_MONITOR_QUEUE_SIZE", workers),
		Timeout:   envDuration("ALERTFLOW_MONITOR_TIMEOUT", 0),
		Reconcile: envDuration("ALERTFLOW_MONITOR_RECONCILE", 5*time.Second),
		Tick:      50 * time.Millisecond,
	}
}

var execute = Probe

// Probe 按目标类型探测。HTTP 与 TCP 的失败是业务采样。
func Probe(ctx context.Context, target system.DueTarget, k8s *kubernetes.K8sObserver) (system.MonitorSample, error) {
	switch target.TargetType {
	case "HTTP":
		return httpprobe.Probe(ctx, target), nil
	case "TCP":
		return tcpprobe.Probe(ctx, target), nil
	case "KUBERNETES":
		if k8s == nil {
			return system.MonitorSample{ServiceID: target.ServiceID, TargetID: target.TargetID}, fmt.Errorf("kubernetes target %s configured but ALERTFLOW_K8S_ENABLED is false", target.TargetID)
		}
		return k8s.Probe(ctx, target)
	default:
		return system.MonitorSample{ServiceID: target.ServiceID, TargetID: target.TargetID}, fmt.Errorf("unsupported target_type %q", target.TargetType)
	}
}

// Run 先停调度，再取消在途探测，并等待 worker 退出。
func Run(ctx context.Context, store Store, k8s *kubernetes.K8sObserver, log *slog.Logger) {
	run(ctx, store, k8s, log, ConfigFromEnv(), nil)
}

func run(ctx context.Context, store Store, k8s *kubernetes.K8sObserver, log *slog.Logger, cfg Config, probed chan<- uuid.UUID) {
	if cfg.Workers <= 0 {
		cfg.Workers = 4
	}
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = cfg.Workers
	}
	if cfg.Reconcile <= 0 {
		cfg.Reconcile = 5 * time.Second
	}
	if cfg.Tick <= 0 {
		cfg.Tick = 50 * time.Millisecond
	}
	sched := scheduler.New()
	jobs := queue.New(cfg.QueueSize)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// worker.RunWithOptions 在 job 边界 recover，这里只负责把 panic 记进日志：
		// 探测 panic 不能再让 worker 静默死亡，否则 4 个 worker 死光即永久停摆。
		worker.RunWithOptions(ctx, cfg.Workers, jobs, func(job queue.Job) {
			interval := job.Target.Interval
			if interval <= 0 {
				interval = time.Second
			}
			// Defer 与 jobs.Done 都走统一出口：无论探测成功、失败、panic 还是 ctx 取消，
			// 都必须归还 target，否则 busy 标记永远不释放，该目标再也不会被调度。
			defer jobs.Done(job.Target.TargetID)
			due := time.Now().Add(interval)
			defer func() {
				// ctx 已取消时探测可能已被打断，这次 Due 没有意义，不回堆。
				if ctx.Err() != nil {
					return
				}
				sched.Defer(job.Target.TargetID, due, job.Version)
			}()
			if sched.Generation(job.Target.TargetID) != job.Generation {
				return
			}
			probeCtx := ctx
			cancel := func() {}
			timeout := job.Target.Timeout
			if cfg.Timeout > 0 {
				timeout = cfg.Timeout
			}
			if timeout > 0 {
				probeCtx, cancel = context.WithTimeout(ctx, timeout)
			}
			sample, err := execute(probeCtx, job.Target, k8s)
			cancel()
			if err == nil && ctx.Err() == nil {
				if _, applyErr := store.ApplyMonitorSample(ctx, sample); applyErr != nil && log != nil {
					log.Error("apply monitor sample failed", "target_id", sample.TargetID, "err", applyErr)
				}
				if probed != nil {
					probed <- job.Target.TargetID
				}
			} else if err != nil && log != nil {
				log.Error("monitor probe failed", "target_id", job.Target.TargetID, "err", err)
			}
		}, worker.Options{OnPanic: func(job queue.Job, recovered any) {
			if log != nil {
				log.Error("monitor probe panicked", "target_id", job.Target.TargetID, "panic", recovered)
			}
		}})
	}()
	if targets, err := store.ListDueTargets(ctx); err == nil {
		sched.Sync(targets, time.Now())
	}
	reconcile := time.NewTicker(cfg.Reconcile)
	tick := time.NewTicker(cfg.Tick)
	defer reconcile.Stop()
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			wg.Wait()
			return
		case <-reconcile.C:
			targets, err := store.ListDueTargets(ctx)
			if err != nil {
				continue
			}
			sched.Sync(targets, time.Now())
		case <-tick.C:
			now := time.Now()
			for {
				item, ok := sched.PopDue(now)
				if !ok {
					break
				}
				accepted, reason := jobs.TryEnqueue(queue.Job{Target: item.Target, Generation: item.Generation, Version: item.Version})
				if accepted {
					continue
				}
				delay := 20 * time.Millisecond
				if reason == "busy" && item.Target.Interval > delay {
					delay = item.Target.Interval
				}
				sched.Defer(item.Target.TargetID, now.Add(delay), item.Version)
				if reason == "full" {
					break
				}
			}
		}
	}
}

func envInt(key string, fallback int) int {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func envDuration(key string, fallback time.Duration) time.Duration {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		return fallback
	}
	return value
}
