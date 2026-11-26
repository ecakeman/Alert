package bootstrap

import (
	"context"
	"log"
	"time"

	apptask "github.com/ecakeman/alert/services/system-core/internal/application/task"
)

// Reconciler 按固定间隔执行恢复扫描；无内部状态累积。
type Reconciler struct {
	tasks    *apptask.Service
	interval time.Duration
}

func NewReconciler(tasks *apptask.Service, interval time.Duration) *Reconciler {
	if interval < time.Second {
		interval = time.Second
	}
	return &Reconciler{tasks: tasks, interval: interval}
}

func (r *Reconciler) Run(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if _, err := r.tasks.Reconcile(ctx, now.UTC()); err != nil {
				log.Printf("reconcile error: %v", err)
			}
		}
	}
}
