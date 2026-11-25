package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	portepisode "github.com/ecakeman/alert/services/system-core/internal/application/port/episode"
	portmonitor "github.com/ecakeman/alert/services/system-core/internal/application/port/monitor"
	portoutbox "github.com/ecakeman/alert/services/system-core/internal/application/port/outbox"
	portService "github.com/ecakeman/alert/services/system-core/internal/application/port/service"
	portshared "github.com/ecakeman/alert/services/system-core/internal/application/port/shared"
	"github.com/ecakeman/alert/services/system-core/internal/contracts"
	"github.com/ecakeman/alert/services/system-core/internal/domain"
)

// Deps 是探测采样落库与故障驱动的依赖。
type Deps struct {
	UOW           portshared.UnitOfWork
	Targets       portService.TargetRepo
	MonitorPoints portmonitor.MonitorPointRepo
	Sources       portService.SourceStateRepo
	Episodes      portepisode.EpisodeRepo
	Services      portService.ServiceRepo
	Outbox        portoutbox.OutboxRepo
	Driver        EpisodeDriver
	Clock         portshared.Clock
	IDs           portshared.IDGen
}

type Service struct {
	deps            Deps
	outOfTxFailures atomic.Int64
}

func New(deps Deps) *Service { return &Service{deps: deps} }

// OutOfTxFailures 返回事务外写失败累计次数（可观测计数）。
func (s *Service) OutOfTxFailures() int64 { return s.outOfTxFailures.Load() }

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

// ApplySample（P0 事务预演）：
//   - 事务内只写本域自产数据：points + outbox + targets.last_check_at；
//   - 事务外更新 targets 状态、sources，并驱动 episode（失败 WARN + 计数，由下一轮补偿）。
func (s *Service) ApplySample(ctx context.Context, serviceID, targetID string, probeGeneration int64, smp Sample) (string, error) {
	var (
		sourceStatus    string
		targetStatus    string
		targetFailures  int
		targetSuccesses int
		targetChanged   bool
		sourceChanged   bool
		skip            bool
	)
	now := s.deps.Clock.Now().UTC()

	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		t, err := s.deps.Targets.Get(ctx, tx, targetID)
		if err != nil {
			return err
		}
		if t.ConfigGeneration != probeGeneration {
			sourceStatus = t.Status
			skip = true
			return nil
		}

		observed := smp.ObservedAt
		if observed.IsZero() {
			observed = now
		}
		if _, err := s.deps.MonitorPoints.Append(ctx, tx, portmonitor.MonitorPoint{
			ServiceID: serviceID, TargetID: targetID, TargetKey: t.TargetKey, TargetType: t.TargetType,
			ObservedAt: observed, ReceivedAt: now,
			CheckStatus: smp.CheckStatus, StatusClass: smp.StatusClass, ErrorClass: smp.ErrorClass,
			StateFingerprint: smp.StateFingerprint, LatencyMS: smp.LatencyMS, ProbeGeneration: probeGeneration,
		}); err != nil {
			return err
		}

		targetStatus, targetFailures, targetSuccesses = nextStatus(t.Status, smp.CheckStatus,
			t.ConsecutiveFailures, t.ConsecutiveSuccesses, t.FailureThreshold, t.RecoveryThreshold)
		targetChanged = targetStatus != t.Status
		if targetChanged {
			if err := s.appendTargetStateChanged(ctx, tx, t, targetStatus, targetFailures, targetSuccesses, observed); err != nil {
				return err
			}
		}

		targets, err := s.deps.Targets.ListByService(ctx, tx, serviceID, false)
		if err != nil {
			return err
		}
		for i := range targets {
			if targets[i].TargetID == targetID {
				targets[i].Status = targetStatus
				targets[i].ConsecutiveFailures = targetFailures
				targets[i].ConsecutiveSuccesses = targetSuccesses
				last := observed
				targets[i].LastCheckAt = &last
			}
		}

		svc, err := s.deps.Services.Get(ctx, tx, serviceID)
		if err != nil {
			return err
		}
		oldSource := "UNKNOWN"
		states, err := s.deps.Sources.ListByService(ctx, tx, serviceID)
		if err != nil {
			return err
		}
		for _, st := range states {
			if st.SourceType == "MONITOR" {
				oldSource = st.Status
			}
		}

		newSource, err := domain.AggregateMonitor(svc.MonitorAggregationMode, monitorTargetHealths(targets))
		if err != nil {
			return err
		}
		sourceChanged = newSource != oldSource
		if sourceChanged {
			if err := s.appendServiceStateChanged(ctx, tx, serviceID, oldSource, newSource, svc.MonitorAggregationMode, targets); err != nil {
				return err
			}
		}
		sourceStatus = newSource

		// P0 无投影表：last_check_at 暂留采样事务；状态/计数在事务外更新。
		return s.deps.Targets.UpdateLastCheckAt(ctx, tx, targetID, now)
	})
	if err != nil {
		return "", err
	}
	if skip {
		return sourceStatus, nil
	}

	if err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		return s.deps.Targets.UpdateProbeResult(ctx, tx, targetID, targetStatus, targetFailures, targetSuccesses, now)
	}); err != nil {
		s.outOfTxFailure("targets.update_probe_result", err)
	}

	if err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		return s.deps.Sources.Upsert(ctx, tx, portService.SourceState{
			ServiceID: serviceID, SourceType: "MONITOR", Status: sourceStatus,
		})
	}); err != nil {
		s.outOfTxFailure("sources.upsert", err)
	}

	// 仅在来源状态变化时驱动 episode；OpenOrReuse/TryResolve 内部对同状态为 no-op。
	if sourceChanged {
		s.driveEpisode(ctx, serviceID, sourceStatus)
	}
	return sourceStatus, nil
}

func (s *Service) driveEpisode(ctx context.Context, serviceID, sourceStatus string) {
	switch sourceStatus {
	case "ABNORMAL":
		if _, err := s.deps.Driver.OpenOrReuse(ctx, serviceID); err != nil {
			s.outOfTxFailure("driver.open_or_reuse", err)
		}
	case "NORMAL":
		var episodeID string
		if err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
			e, err := s.deps.Episodes.GetActiveByService(ctx, tx, serviceID)
			if err != nil {
				return nil
			}
			episodeID = e.EpisodeID
			return nil
		}); err != nil {
			s.outOfTxFailure("episodes.get_active", err)
		}
		if episodeID != "" {
			if _, _, err := s.deps.Driver.TryResolve(ctx, episodeID); err != nil {
				s.outOfTxFailure("driver.try_resolve", err)
			}
		}
	}
}

func (s *Service) outOfTxFailure(op string, err error) {
	s.outOfTxFailures.Add(1)
	slog.Default().WarnContext(context.Background(), "monitor out-of-tx write failed",
		"op", op, "reason", err.Error())
}

type targetStatePayload struct {
	ServiceID            string `json:"service_id"`
	TargetID             string `json:"target_id"`
	Status               string `json:"status"`
	ConsecutiveFailures  int    `json:"consecutive_failures"`
	ConsecutiveSuccesses int    `json:"consecutive_successes"`
	LastCheckAt          string `json:"last_check_at"`
}

type serviceStatePayload struct {
	ServiceID          string `json:"service_id"`
	SourceStatus       string `json:"source_status"`
	Status             string `json:"status"`
	AggregationMode    string `json:"aggregation_mode"`
	TargetHealthDigest string `json:"target_health_digest"`
}

func (s *Service) appendTargetStateChanged(ctx context.Context, tx portshared.Tx, t portService.MonitorTarget, status string, failures, successes int, observed time.Time) error {
	return s.appendEvent(ctx, tx, contracts.EventMonitorTargetStateChanged, "monitor_target", t.TargetID, targetStatePayload{
		ServiceID:            t.ServiceID,
		TargetID:             t.TargetID,
		Status:               status,
		ConsecutiveFailures:  failures,
		ConsecutiveSuccesses: successes,
		LastCheckAt:          observed.UTC().Format(time.RFC3339),
	})
}

func (s *Service) appendServiceStateChanged(ctx context.Context, tx portshared.Tx, serviceID, oldStatus, newStatus, mode string, targets []portService.MonitorTarget) error {
	return s.appendEvent(ctx, tx, contracts.EventMonitorServiceStateChanged, "service", serviceID, serviceStatePayload{
		ServiceID:          serviceID,
		SourceStatus:       oldStatus,
		Status:             newStatus,
		AggregationMode:    mode,
		TargetHealthDigest: targetHealthDigest(targets),
	})
}

func (s *Service) appendEvent(ctx context.Context, tx portshared.Tx, eventType, aggregateType, aggregateID string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	key := eventType + ":" + aggregateID + ":" + s.deps.IDs.NewID()
	return s.deps.Outbox.Append(ctx, tx, contracts.Event{
		EventID: s.deps.IDs.NewID(), EventType: eventType, SchemaVersion: 1,
		AggregateType: aggregateType, AggregateID: aggregateID,
		OccurredAt: s.deps.Clock.Now().UTC(), Payload: body,
	}, &key)
}

func targetHealthDigest(targets []portService.MonitorTarget) string {
	var normal, abnormal, unknown int
	for _, t := range targets {
		switch t.Status {
		case "NORMAL":
			normal++
		case "ABNORMAL":
			abnormal++
		default:
			unknown++
		}
	}
	return fmt.Sprintf("normal=%d,abnormal=%d,unknown=%d", normal, abnormal, unknown)
}

func monitorTargetHealths(targets []portService.MonitorTarget) []domain.TargetHealth {
	out := make([]domain.TargetHealth, 0, len(targets))
	for _, t := range targets {
		out = append(out, domain.TargetHealth{
			Required: t.IsRequired,
			Enabled:  t.Enabled,
			Deleted:  false, // 聚合输入已由 SQL 过滤软删除目标
			Status:   t.Status,
		})
	}
	return out
}
