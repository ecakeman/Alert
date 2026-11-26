package probe

import (
	"context"
	"sync"
	"time"

	portmonitor "github.com/ecakeman/alert/services/system-core/internal/application/port/monitor"
	portService "github.com/ecakeman/alert/services/system-core/internal/application/port/service"
	portshared "github.com/ecakeman/alert/services/system-core/internal/application/port/shared"
)

// SampleHandler 由 application 注入，负责把探测结果落库并驱动故障。
// 入参为调度快照（含 ConfigGeneration），infrastructure 不持有仓储。
type SampleHandler func(ctx context.Context, target portmonitor.TargetSnapshot, sample Sample) error

type EngineDeps struct {
	Source    portmonitor.TargetScheduleSource
	HTTP      Prober
	TCP       Prober
	Handler   SampleHandler
	Workers   int
	Capacity  int
	Clock     portshared.Clock
	ScanEvery time.Duration
}

type Engine struct {
	deps     EngineDeps
	inflight sync.Map
}

func NewEngine(deps EngineDeps) *Engine {
	if deps.Workers <= 0 {
		deps.Workers = 8
	}
	if deps.Capacity <= 0 {
		deps.Capacity = 1024
	}
	if deps.ScanEvery <= 0 {
		deps.ScanEvery = time.Second
	}
	return &Engine{deps: deps}
}

type job struct {
	target portmonitor.TargetSnapshot
}

// Run 调度 + worker 池，直到 ctx 取消；取消时关闭队列并等待在途探测结束。
func (e *Engine) Run(ctx context.Context) error {
	queue := make(chan job, e.deps.Capacity)
	var wg sync.WaitGroup

	for i := 0; i < e.deps.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case j, ok := <-queue:
					if !ok {
						return
					}
					e.probeOne(ctx, j)
				}
			}
		}()
	}

	ticker := time.NewTicker(e.deps.ScanEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			close(queue)
			wg.Wait()
			return nil
		case <-ticker.C:
			e.schedule(ctx, queue)
		}
	}
}

func (e *Engine) schedule(ctx context.Context, queue chan job) {
	// 调度编排（读启用目标 + 到期过滤）在应用侧 DueTargets；这里只做执行机制。
	snapshots, err := e.deps.Source.DueTargets(ctx, e.deps.Clock.Now().UTC())
	if err != nil {
		return
	}
	for _, snap := range snapshots {
		if _, loaded := e.inflight.LoadOrStore(snap.TargetID, struct{}{}); loaded {
			continue
		}
		select {
		case queue <- job{target: snap}:
		default:
			e.inflight.Delete(snap.TargetID) // 队列满，下轮重排，不丢
		}
	}
}

func (e *Engine) probeOne(ctx context.Context, j job) {
	defer e.inflight.Delete(j.target.TargetID)
	defer func() { _ = recover() }()

	p := e.deps.HTTP
	if j.target.TargetType == "TCP" {
		p = e.deps.TCP
	}
	smp := p.Probe(ctx, toMonitorTarget(j.target))
	_ = e.deps.Handler(ctx, j.target, smp)
}

// toMonitorTarget 把调度快照适配为 prober 入参（prober 只读地址/类型/超时）。
func toMonitorTarget(snap portmonitor.TargetSnapshot) portService.MonitorTarget {
	return portService.MonitorTarget{
		TargetID:             snap.TargetID,
		ServiceID:            snap.ServiceID,
		TargetKey:            snap.TargetKey,
		TargetType:           snap.TargetType,
		Address:              snap.Address,
		Config:               snap.Config,
		IntervalMS:           snap.IntervalMS,
		TimeoutMS:            snap.TimeoutMS,
		FailureThreshold:     snap.FailureThreshold,
		RecoveryThreshold:    snap.RecoveryThreshold,
		IsRequired:           snap.IsRequired,
		Status:               snap.Status,
		ConsecutiveFailures:  snap.ConsecutiveFailures,
		ConsecutiveSuccesses: snap.ConsecutiveSuccesses,
		LastCheckAt:          snap.LastCheckAt,
		ConfigGeneration:     snap.ConfigGeneration,
	}
}
