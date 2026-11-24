package monitor

import (
	"context"
	"time"

	"github.com/ecakeman/alert/internal/application/port"
	"github.com/ecakeman/alert/internal/domain"
)

// Deps 是探测采样落库与故障驱动的依赖。
type Deps struct {
	UOW           port.UnitOfWork
	Targets       port.TargetRepo
	MonitorPoints port.MonitorPointRepo
	Sources       port.SourceStateRepo
	Episodes      port.EpisodeRepo
	Services      port.ServiceRepo
	Driver        EpisodeDriver
	Clock         port.Clock
}

type Service struct{ deps Deps }

func New(deps Deps) *Service { return &Service{deps: deps} }

// Sample 是一次探测结果（由 engine 从 probe.Sample 转换）。
type Sample struct {
	CheckStatus      string
	StatusClass      string
	ErrorClass       string
	LatencyMS        *int
	StateFingerprint string
	ObservedAt       time.Time
	ProbeGeneration  int64
}

// ApplySample：写监控点 → 聚合计数/状态 → 重算 MONITOR 来源 → 驱动故障/恢复。
func (s *Service) ApplySample(ctx context.Context, serviceID, targetID string, probeGeneration int64, smp Sample) (string, error) {
	var sourceStatus string
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx port.Tx) error {
		t, err := s.deps.Targets.Get(ctx, tx, targetID)
		if err != nil {
			return err
		}
		if t.ConfigGeneration != probeGeneration {
			sourceStatus = t.Status
			return nil
		}

		observed := smp.ObservedAt
		if observed.IsZero() {
			observed = s.deps.Clock.Now().UTC()
		}
		if _, err := s.deps.MonitorPoints.Append(ctx, tx, port.MonitorPoint{
			ServiceID: serviceID, TargetID: targetID, TargetKey: t.TargetKey, TargetType: t.TargetType,
			ObservedAt: observed, ReceivedAt: s.deps.Clock.Now().UTC(),
			CheckStatus: smp.CheckStatus, StatusClass: smp.StatusClass, ErrorClass: smp.ErrorClass,
			StateFingerprint: smp.StateFingerprint, LatencyMS: smp.LatencyMS, ProbeGeneration: probeGeneration,
		}); err != nil {
			return err
		}

		newStatus, failures, successes := nextStatus(t.Status, smp.CheckStatus,
			t.ConsecutiveFailures, t.ConsecutiveSuccesses, t.FailureThreshold, t.RecoveryThreshold)
		if err := s.deps.Targets.UpdateProbeResult(ctx, tx, targetID, newStatus, failures, successes, s.deps.Clock.Now().UTC()); err != nil {
			return err
		}

		sourceStatus, err = s.recomputeSource(ctx, tx, serviceID)
		return err
	})
	if err != nil {
		return "", err
	}

	// 驱动故障（事务外，复用 episode 用例的独立事务）。
	if sourceStatus == "ABNORMAL" {
		_, err := s.deps.Driver.OpenOrReuse(ctx, serviceID)
		return sourceStatus, err
	}
	if sourceStatus == "NORMAL" {
		var episodeID string
		_ = s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx port.Tx) error {
			e, err := s.deps.Episodes.GetActiveByService(ctx, tx, serviceID)
			if err == nil {
				episodeID = e.EpisodeID
			}
			return nil
		})
		if episodeID != "" {
			_, _, err := s.deps.Driver.TryResolve(ctx, episodeID)
			return sourceStatus, err
		}
	}
	return sourceStatus, nil
}

func (s *Service) recomputeSource(ctx context.Context, tx port.Tx, serviceID string) (string, error) {
	targets, err := s.deps.Targets.ListByService(ctx, tx, serviceID, false)
	if err != nil {
		return "", err
	}
	svc, err := s.deps.Services.Get(ctx, tx, serviceID)
	if err != nil {
		return "", err
	}
	status, err := domain.AggregateMonitor(svc.MonitorAggregationMode, monitorTargetHealths(targets))
	if err != nil {
		return "", err
	}
	if err := s.deps.Sources.Upsert(ctx, tx, port.SourceState{
		ServiceID: serviceID, SourceType: "MONITOR", Status: status,
	}); err != nil {
		return "", err
	}
	return status, nil
}

func monitorTargetHealths(targets []port.MonitorTarget) []domain.TargetHealth {
	out := make([]domain.TargetHealth, 0, len(targets))
	for _, t := range targets {
		out = append(out, domain.TargetHealth{
			Required: t.IsRequired,
			Enabled:  t.Enabled,
			Deleted:  t.DeletedAt != nil,
			Status:   t.Status,
		})
	}
	return out
}
