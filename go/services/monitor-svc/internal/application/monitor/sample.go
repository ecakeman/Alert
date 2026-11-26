package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	portmonitor "github.com/ecakeman/alert/services/monitor-svc/internal/application/port/monitor"
	portoutbox "github.com/ecakeman/alert/services/monitor-svc/internal/application/port/outbox"
	"github.com/ecakeman/alert/services/monitor-svc/internal/application/port/projection"
	portshared "github.com/ecakeman/alert/services/monitor-svc/internal/application/port/shared"
	portstate "github.com/ecakeman/alert/services/monitor-svc/internal/application/port/state"
	"github.com/ecakeman/alert/services/monitor-svc/internal/contracts"
	"github.com/ecakeman/alert/services/monitor-svc/internal/domain"
)

// Deps 是采样落库的依赖；全部为本域私有存储，无跨域写入。
type Deps struct {
	UOW        portshared.UnitOfWork
	Points     portmonitor.MonitorPointRepo
	Projection projection.Repo
	States     portstate.Repo
	Outbox     portoutbox.OutboxRepo
	Clock      portshared.Clock
	IDs        portshared.IDGen
}

type Service struct {
	deps            Deps
	outOfTxFailures atomic.Int64
}

func New(deps Deps) *Service { return &Service{deps: deps} }

// OutOfTxFailures 返回事务外失败累计次数（当前采样无事务外写，保留可观测计数）。
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

// DueTargets 从投影表读取到期目标（P3 起调度唯一来源）。
func (s *Service) DueTargets(ctx context.Context, now time.Time) ([]portmonitor.TargetSnapshot, error) {
	var out []portmonitor.TargetSnapshot
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		targets, err := s.deps.Projection.ListDue(ctx, tx, now)
		if err != nil {
			return err
		}
		for _, t := range targets {
			out = append(out, portmonitor.TargetSnapshot{
				TargetID: t.TargetID, ServiceID: t.ServiceID, TargetKey: t.TargetKey,
				TargetType: t.TargetType, Address: t.Address, IntervalMS: t.IntervalMS, TimeoutMS: t.TimeoutMS,
				FailureThreshold: t.FailureThreshold, RecoveryThreshold: t.RecoveryThreshold,
				IsRequired: t.IsRequired, ConfigGeneration: t.ConfigGeneration,
			})
		}
		return nil
	})
	return out, err
}

// ApplySample：事务内写 points + 自产状态 + outbox（仅状态变化时）；无跨域写。
func (s *Service) ApplySample(ctx context.Context, serviceID, targetID string, probeGeneration int64, smp Sample) (string, error) {
	now := s.deps.Clock.Now().UTC()
	observed := smp.ObservedAt
	if observed.IsZero() {
		observed = now
	}

	var newSource string

	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		t, err := s.deps.Projection.Get(ctx, tx, targetID)
		if err != nil {
			return err
		}
		if t.DeletedAt != nil {
			return nil
		}
		if t.ConfigGeneration != probeGeneration {
			// 配置已换代：丢弃过期采样（design 2.1.3.2 T1）。
			return nil
		}

		if _, err := s.deps.Points.Append(ctx, tx, portmonitor.MonitorPoint{
			ServiceID: serviceID, TargetID: targetID, TargetKey: t.TargetKey, TargetType: t.TargetType,
			ObservedAt: observed, ReceivedAt: now,
			CheckStatus: smp.CheckStatus, StatusClass: smp.StatusClass, ErrorClass: smp.ErrorClass,
			StateFingerprint: smp.StateFingerprint, LatencyMS: smp.LatencyMS, ProbeGeneration: probeGeneration,
		}); err != nil {
			return err
		}

		old, err := s.deps.States.GetTarget(ctx, tx, targetID)
		if err != nil {
			return err
		}
		status, failures, successes := nextStatus(old.Status, smp.CheckStatus,
			old.ConsecutiveFailures, old.ConsecutiveSuccesses, t.FailureThreshold, t.RecoveryThreshold)
		if err := s.deps.States.UpsertTarget(ctx, tx, portstate.TargetState{
			TargetID: targetID, ServiceID: serviceID, Status: status,
			ConsecutiveFailures: failures, ConsecutiveSuccesses: successes,
			LastCheckAt: &observed,
		}, now); err != nil {
			return err
		}
		if err := s.deps.Projection.UpdateLastProbeAt(ctx, tx, targetID, observed); err != nil {
			return err
		}

		oldSource, err := s.deps.States.GetSource(ctx, tx, serviceID)
		if err != nil {
			return err
		}
		projs, err := s.deps.Projection.ListByService(ctx, tx, serviceID, false)
		if err != nil {
			return err
		}
		states, err := s.deps.States.ListTargets(ctx, tx, serviceID)
		if err != nil {
			return err
		}
		newSource, err = domain.AggregateMonitor(t.MonitorAggregationMode, healths(projs, states))
		if err != nil {
			return err
		}
		if err := s.deps.States.UpsertSource(ctx, tx, portstate.SourceState{
			ServiceID: serviceID, Status: newSource, AggregationMode: t.MonitorAggregationMode,
		}, now); err != nil {
			return err
		}

		if status != old.Status {
			evt, err := s.buildTargetEvent(targetID, serviceID, status, failures, successes, observed)
			if err != nil {
				return err
			}
			dedupe := evt.EventType + ":" + evt.AggregateID + ":" + s.deps.IDs.NewID()
			if err := s.deps.Outbox.Append(ctx, tx, *evt, &dedupe); err != nil {
				return err
			}
		}
		if newSource != oldSource.Status {
			evt, err := s.buildSourceEvent(serviceID, oldSource.Status, newSource, t.MonitorAggregationMode, projs, states)
			if err != nil {
				return err
			}
			dedupe := evt.EventType + ":" + evt.AggregateID + ":" + s.deps.IDs.NewID()
			if err := s.deps.Outbox.Append(ctx, tx, *evt, &dedupe); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return newSource, nil
}

func (s *Service) outOfTxFailure(op string, err error) {
	s.outOfTxFailures.Add(1)
	slog.Default().WarnContext(context.Background(), "monitor out-of-tx write failed",
		"op", op, "reason", err.Error())
}

func healths(projs []projection.Target, states []portstate.TargetState) []domain.TargetHealth {
	byTarget := make(map[string]string, len(states))
	for _, st := range states {
		byTarget[st.TargetID] = st.Status
	}
	out := make([]domain.TargetHealth, 0, len(projs))
	for _, p := range projs {
		out = append(out, domain.TargetHealth{
			Required: p.IsRequired, Enabled: p.Enabled, Deleted: p.DeletedAt != nil,
			Status: byTarget[p.TargetID],
		})
	}
	return out
}

func (s *Service) buildTargetEvent(targetID, serviceID, status string, failures, successes int, observed time.Time) (*contracts.Event, error) {
	body, err := json.Marshal(map[string]any{
		"service_id":            serviceID,
		"target_id":             targetID,
		"status":                status,
		"consecutive_failures":  failures,
		"consecutive_successes": successes,
		"last_check_at":         observed.UTC().Format(time.RFC3339),
	})
	if err != nil {
		return nil, err
	}
	return &contracts.Event{
		EventID: s.deps.IDs.NewID(), EventType: contracts.EventMonitorTargetStateChanged, SchemaVersion: 1,
		AggregateType: "monitor_target", AggregateID: targetID,
		OccurredAt: s.deps.Clock.Now().UTC(), Payload: body,
	}, nil
}

func (s *Service) buildSourceEvent(serviceID, oldStatus, newStatus, mode string, projs []projection.Target, states []portstate.TargetState) (*contracts.Event, error) {
	var normal, abnormal, unknown int
	for _, h := range healths(projs, states) {
		switch h.Status {
		case "NORMAL":
			normal++
		case "ABNORMAL":
			abnormal++
		default:
			unknown++
		}
	}
	body, err := json.Marshal(map[string]any{
		"service_id":           serviceID,
		"source_status":        oldStatus,
		"status":               newStatus,
		"aggregation_mode":     mode,
		"target_health_digest": fmt.Sprintf("normal=%d,abnormal=%d,unknown=%d", normal, abnormal, unknown),
	})
	if err != nil {
		return nil, err
	}
	return &contracts.Event{
		EventID: s.deps.IDs.NewID(), EventType: contracts.EventMonitorServiceStateChanged, SchemaVersion: 1,
		AggregateType: "service", AggregateID: serviceID,
		OccurredAt: s.deps.Clock.Now().UTC(), Payload: body,
	}, nil
}