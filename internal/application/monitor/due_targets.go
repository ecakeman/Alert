package monitor

import (
	"context"
	"time"

	portmonitor "github.com/ecakeman/alert/internal/application/port/monitor"
	portService "github.com/ecakeman/alert/internal/application/port/service"
	portshared "github.com/ecakeman/alert/internal/application/port/shared"
)

// snapshotFromTarget 把启用目标映射为调度快照。
func snapshotFromTarget(t portService.MonitorTarget) portmonitor.TargetSnapshot {
	return portmonitor.TargetSnapshot{
		TargetID:             t.TargetID,
		ServiceID:            t.ServiceID,
		TargetKey:            t.TargetKey,
		TargetType:           t.TargetType,
		Address:              t.Address,
		Config:               t.Config,
		IntervalMS:           t.IntervalMS,
		TimeoutMS:            t.TimeoutMS,
		FailureThreshold:     t.FailureThreshold,
		RecoveryThreshold:    t.RecoveryThreshold,
		IsRequired:           t.IsRequired,
		Status:               t.Status,
		ConsecutiveFailures:  t.ConsecutiveFailures,
		ConsecutiveSuccesses: t.ConsecutiveSuccesses,
		LastCheckAt:          t.LastCheckAt,
		ConfigGeneration:     t.ConfigGeneration,
	}
}

// due 判断目标是否到期：LastCheckAt 为空视为到期；否则 LastCheckAt+IntervalMS <= now。
func due(t portService.MonitorTarget, now time.Time) bool {
	interval := time.Duration(t.IntervalMS) * time.Millisecond
	if interval <= 0 {
		interval = 30 * time.Second
	}
	if t.LastCheckAt == nil {
		return true
	}
	return !t.LastCheckAt.Add(interval).After(now)
}

// DueTargets 读取启用目标并返回到期待探测的快照。
// 调度编排属应用侧；infrastructure 只保留执行机制（worker/prober）。
func (s *Service) DueTargets(ctx context.Context, now time.Time) ([]portmonitor.TargetSnapshot, error) {
	var out []portmonitor.TargetSnapshot
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		targets, err := s.deps.Targets.ListEnabled(ctx, tx)
		if err != nil {
			return err
		}
		for _, t := range targets {
			if !due(t, now) {
				continue
			}
			out = append(out, snapshotFromTarget(t))
		}
		return nil
	})
	return out, err
}
