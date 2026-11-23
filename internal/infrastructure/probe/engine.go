package probe

import (
	"context"
	"sync"
	"time"

	"github.com/ecakeman/alert/internal/application/port"
)

// SampleHandler 由 application 注入，负责把探测结果落库并驱动故障。
type SampleHandler func(ctx context.Context, target port.MonitorTarget, sample Sample) error

type EngineDeps struct {
	UOW       port.UnitOfWork
	Targets   port.TargetRepo
	HTTP      Prober
	TCP       Prober
	Handler   SampleHandler
	Workers   int
	Capacity  int
	Clock     port.Clock
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
	target port.MonitorTarget
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
	var targets []port.MonitorTarget
	err := e.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx port.Tx) error {
		var err error
		targets, err = e.deps.Targets.ListEnabled(ctx, tx)
		return err
	})
	if err != nil {
		return
	}
	now := e.deps.Clock.Now().UTC()
	for _, t := range targets {
		interval := time.Duration(t.IntervalMS) * time.Millisecond
		if interval <= 0 {
			interval = 30 * time.Second
		}
		if t.LastCheckAt != nil && t.LastCheckAt.Add(interval).After(now) {
			continue
		}
		if _, loaded := e.inflight.LoadOrStore(t.TargetID, struct{}{}); loaded {
			continue
		}
		select {
		case queue <- job{target: t}:
		default:
			e.inflight.Delete(t.TargetID) // 队列满，下轮重排，不丢
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
	smp := p.Probe(ctx, j.target)
	_ = e.deps.Handler(ctx, j.target, smp)
}
