package service

import (
	"context"
	"time"

	portService "github.com/ecakeman/alert/services/service-registry/internal/application/port/service"
	portshared "github.com/ecakeman/alert/services/service-registry/internal/application/port/shared"
)

// GetTarget 返回单个目标（单体探测调度用）。
func (s *Service) GetTarget(ctx context.Context, targetID string) (portService.MonitorTarget, error) {
	var out portService.MonitorTarget
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		var err error
		out, err = s.deps.Targets.Get(ctx, tx, targetID)
		return err
	})
	return out, err
}

// ListEnabledTargets 返回全部启用目标（单体探测 engine 的输入）。
func (s *Service) ListEnabledTargets(ctx context.Context) ([]portService.MonitorTarget, error) {
	var out []portService.MonitorTarget
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		var err error
		out, err = s.deps.Targets.ListEnabled(ctx, tx)
		return err
	})
	return out, err
}

// ApplyProbeResult 是 P2 临时命令：单体 worker 探测结果回写；status 为空时只更新时间。
func (s *Service) ApplyProbeResult(ctx context.Context, targetID, status string, failures, successes int, lastCheckAt time.Time) error {
	return s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		if status == "" {
			return s.deps.Targets.UpdateLastCheckAt(ctx, tx, targetID, lastCheckAt)
		}
		return s.deps.Targets.UpdateProbeResult(ctx, tx, targetID, status, failures, successes, lastCheckAt)
	})
}

// UpsertSourceState 是 P2 临时命令：单体 APP/MONITOR 来源状态回写。
func (s *Service) UpsertSourceState(ctx context.Context, state portService.SourceState) error {
	return s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		return s.deps.Sources.Upsert(ctx, tx, state)
	})
}
