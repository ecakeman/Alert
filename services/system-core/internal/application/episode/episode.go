package episode

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/ecakeman/alert/services/system-core/internal/application/apperr"
	portepisode "github.com/ecakeman/alert/services/system-core/internal/application/port/episode"
	portmonitor "github.com/ecakeman/alert/services/system-core/internal/application/port/monitor"
	portoutbox "github.com/ecakeman/alert/services/system-core/internal/application/port/outbox"
	portService "github.com/ecakeman/alert/services/system-core/internal/application/port/service"
	portshared "github.com/ecakeman/alert/services/system-core/internal/application/port/shared"
	porttask "github.com/ecakeman/alert/services/system-core/internal/application/port/task"
	"github.com/ecakeman/alert/services/system-core/internal/contracts"
	"github.com/ecakeman/alert/services/system-core/internal/domain"
)

type Deps struct {
	UOW           portshared.UnitOfWork
	Episodes      portepisode.EpisodeRepo
	InputVersions portepisode.InputVersionRepo
	AppEvents     portepisode.AppEventRepo
	MonitorPoints portmonitor.MonitorPointRepo
	Sources       portService.SourceStateRepo
	Services      portService.ServiceRepo
	Tasks         porttask.TaskRepo
	Artifacts     porttask.ArtifactRepo
	Outbox        portoutbox.OutboxRepo
	Clock         portshared.Clock
	IDs           portshared.IDGen
}

type Service struct{ deps Deps }

func New(deps Deps) *Service { return &Service{deps: deps} }

func isAppCode(err error, code string) bool {
	var ae *apperr.Error
	return errors.As(err, &ae) && ae.Code == code
}

// OpenOrReuse：同服务有 ACTIVE 则复用，否则创建（INV-01）。
func (s *Service) OpenOrReuse(ctx context.Context, serviceID string) (portepisode.Episode, bool, error) {
	var ep portepisode.Episode
	var created bool
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		active, err := s.deps.Episodes.GetActiveByService(ctx, tx, serviceID)
		activeExists := err == nil
		if err != nil && !isAppCode(err, apperr.CodeNotFound) {
			return err
		}
		decision := domain.DecideIncidentOnAbnormal(activeExists, active.EpisodeID)
		if decision.Kind == domain.IncidentReuse {
			ep = active
			return nil
		}
		now := s.deps.Clock.Now().UTC()
		ep = portepisode.Episode{
			EpisodeID: s.deps.IDs.NewID(), ServiceID: serviceID, Status: "ACTIVE",
			StartedBy: "MONITOR", StartedAt: now, UpdatedAt: now,
		}
		if err := s.deps.Episodes.Create(ctx, tx, ep); err != nil {
			return err
		}
		created = true
		return s.appendEvent(ctx, tx, "episode.status.changed.v1", "episode", ep.EpisodeID,
			map[string]string{"episode_id": ep.EpisodeID, "status": "ACTIVE"})
	})
	return ep, created, err
}

func (s *Service) List(ctx context.Context, f portepisode.EpisodeFilter) ([]portepisode.Episode, error) {
	var out []portepisode.Episode
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		var err error
		out, err = s.deps.Episodes.List(ctx, tx, f)
		return err
	})
	return out, err
}

func (s *Service) Get(ctx context.Context, episodeID string) (EpisodeView, error) {
	var view EpisodeView
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		ep, err := s.deps.Episodes.Get(ctx, tx, episodeID)
		if err != nil {
			return err
		}
		sources, err := s.deps.Sources.ListByService(ctx, tx, ep.ServiceID)
		if err != nil {
			return err
		}
		tasks, err := s.deps.Tasks.ListActiveByEpisode(ctx, tx, episodeID)
		if err != nil {
			return err
		}
		svc, err := s.deps.Services.Get(ctx, tx, ep.ServiceID)
		if err != nil {
			return err
		}
		view = EpisodeView{Episode: ep, Sources: sources, ServiceName: svc.Name}
		if len(tasks) > 0 {
			view.ActiveTask = &tasks[0]
		}
		return nil
	})
	return view, err
}

// AppendAppEvent：同键同内容幂等、同键不同内容 409、新键追加（INV-05）。
func (s *Service) AppendAppEvent(ctx context.Context, serviceID string, in AppEventInput) (AppendEventResult, error) {
	if in.ProducerID == "" || in.EventID == "" {
		return AppendEventResult{}, apperr.Validation("producer_id and event_id are required")
	}
	var res AppendEventResult
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		existing, err := s.deps.AppEvents.GetByIdentity(ctx, tx, serviceID, in.ProducerID, in.EventID)
		var existingPtr *portepisode.AppEvent
		if err == nil {
			existingPtr = &existing
		} else if !isAppCode(err, apperr.CodeNotFound) {
			return err
		}

		incoming := portepisode.AppEvent{
			ServiceID: serviceID, ProducerID: in.ProducerID, EventID: in.EventID,
			EventKind: in.EventKind, HealthStatus: in.HealthStatus, Payload: in.Payload,
			ContentHash: contentHash(in.EventKind, in.HealthStatus, in.Payload),
			EventTime:   in.EventTime, ReceivedAt: s.deps.Clock.Now().UTC(),
		}

		decision := domain.DecideAppEvent(toDomainEvent(existingPtr), *toDomainEvent(&incoming))
		switch decision.Decision {
		case domain.AppEventDuplicate:
			res = AppendEventResult{Event: existing, Appended: false}
			return nil
		case domain.AppEventConflict:
			return apperr.New(apperr.CodeEventIDConflict, "event id conflict")
		}

		if _, err := s.deps.AppEvents.Append(ctx, tx, incoming); err != nil {
			return err
		}
		stored, err := s.deps.AppEvents.GetByIdentity(ctx, tx, serviceID, in.ProducerID, in.EventID)
		if err != nil {
			return err
		}
		res = AppendEventResult{Event: stored, Appended: true}
		return s.appendEvent(ctx, tx, "episode.input.appended.v1", "service", serviceID,
			map[string]string{"service_id": serviceID, "event_id": in.EventID})
	})
	return res, err
}

// AppendInputVersion：仅在指纹变化时新增版本，并推进水位线（INV-04）。
func (s *Service) AppendInputVersion(ctx context.Context, episodeID string) (bool, error) {
	var appended bool
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		ep, err := s.deps.Episodes.Get(ctx, tx, episodeID)
		if err != nil {
			return err
		}

		latestSeq, err := s.deps.MonitorPoints.LatestSeqByService(ctx, tx, ep.ServiceID)
		if err != nil {
			return err
		}
		points, err := s.deps.MonitorPoints.ListByBound(ctx, tx, ep.ServiceID, 0, latestSeq, 1000)
		if err != nil {
			return err
		}
		events, err := s.deps.AppEvents.ListByBound(ctx, tx, ep.ServiceID, 0, int64(1)<<62, 1000)
		if err != nil {
			return err
		}

		projection := buildProjection(points)
		canon := buildCanon(events)
		hash, err := domain.Fingerprint(projection, canon)
		if err != nil {
			return err
		}
		if hash == ep.InputHash {
			return nil
		}

		newVersion := ep.InputVersion + 1
		projectionJSON, err := json.Marshal(projection)
		if err != nil {
			return err
		}
		if _, err := s.deps.InputVersions.AppendIfChanged(ctx, tx, portepisode.InputVersion{
			EpisodeID: episodeID, InputVersion: newVersion, InputHash: hash,
			MonitorProjection: projectionJSON, MonitorWatermark: latestSeq,
			AppEventWatermark: int64(1) << 62,
		}); err != nil {
			return err
		}
		if err := s.deps.Episodes.TouchInputVersion(ctx, tx, episodeID, newVersion, hash, latestSeq, int64(1)<<62); err != nil {
			return err
		}
		appended = true
		return s.appendEvent(ctx, tx, "episode.input.appended.v1", "episode", episodeID,
			map[string]any{"episode_id": episodeID, "input_version": newVersion})
	})
	return appended, err
}

// TryResolve：满足全部解除条件才置 RESOLVED（INV-03）。
// P0 预演：判定数据在事务外读取，写侧用 CAS（仍 ACTIVE 且 updated_at 未变）避免错误解除。
func (s *Service) TryResolve(ctx context.Context, episodeID string) (bool, []string, error) {
	var (
		ep          portepisode.Episode
		sources     []portService.SourceState
		activeTasks []porttask.AgentTask
	)
	if err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		var err error
		ep, err = s.deps.Episodes.Get(ctx, tx, episodeID)
		if err != nil {
			return err
		}
		sources, err = s.deps.Sources.ListByService(ctx, tx, ep.ServiceID)
		if err != nil {
			return err
		}
		activeTasks, err = s.deps.Tasks.ListActiveByEpisode(ctx, tx, episodeID)
		return err
	}); err != nil {
		return false, nil, apperr.DependencyUnavailable("resolve precheck unavailable")
	}

	monitorNormal, appNormal := false, false
	for _, st := range sources {
		switch st.SourceType {
		case "MONITOR":
			monitorNormal = st.Status == "NORMAL"
		case "APP":
			appNormal = st.Status == "NORMAL"
		}
	}
	finalProcessed := ep.FinalInputVersion > 0 && ep.ProcessedInputVersion == ep.FinalInputVersion
	ok, miss := domain.CanResolve(domain.ResolveInput{
		MonitorNormal:       monitorNormal,
		AppNormal:           appNormal,
		FinalInputProcessed: finalProcessed,
		ActiveTaskExists:    len(activeTasks) > 0,
	})
	missing := make([]string, 0, len(miss))
	for _, m := range miss {
		missing = append(missing, string(m))
	}
	if !ok {
		return false, missing, nil
	}

	var resolved bool
	now := s.deps.Clock.Now().UTC()
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		applied, err := s.deps.Episodes.ResolveIfUnchanged(ctx, tx, episodeID, ep.UpdatedAt, now)
		if err != nil {
			return err
		}
		if !applied {
			slog.Default().WarnContext(ctx, "resolve skipped by CAS conflict", "episode_id", episodeID)
			return nil
		}
		resolved = true
		return s.appendEvent(ctx, tx, "episode.status.changed.v1", "episode", episodeID,
			map[string]string{"episode_id": episodeID, "status": "RESOLVED"})
	})
	if err != nil {
		return false, missing, err
	}
	return resolved, missing, nil
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

func toDomainEvent(e *portepisode.AppEvent) *domain.AppEvent {
	if e == nil {
		return nil
	}
	return &domain.AppEvent{
		ServiceID: e.ServiceID, ProducerID: e.ProducerID, EventID: e.EventID,
		EventKind: e.EventKind, HealthStatus: e.HealthStatus,
		Payload: json.RawMessage(e.Payload), ContentHash: e.ContentHash,
	}
}

func contentHash(kind, health string, payload []byte) string {
	h := sha256.New()
	h.Write([]byte(kind))
	h.Write([]byte{0})
	h.Write([]byte(health))
	h.Write([]byte{0})
	h.Write(payload)
	return hex.EncodeToString(h.Sum(nil))
}

func buildProjection(points []portmonitor.MonitorPoint) []domain.MonitorField {
	latest := map[string]portmonitor.MonitorPoint{}
	for _, p := range points {
		latest[p.TargetKey] = p
	}
	out := make([]domain.MonitorField, 0, len(latest))
	for _, p := range latest {
		out = append(out, domain.MonitorField{
			TargetKey: p.TargetKey, TargetType: p.TargetType, Status: p.CheckStatus,
			StatusClass: p.StatusClass, ErrorClass: p.ErrorClass, StateFingerprint: p.StateFingerprint,
		})
	}
	return out
}

func buildCanon(events []portepisode.AppEvent) []domain.AppCanon {
	out := make([]domain.AppCanon, 0, len(events))
	for _, e := range events {
		out = append(out, domain.AppCanon{
			EventKind: e.EventKind, HealthStatus: e.HealthStatus, Payload: json.RawMessage(e.Payload),
		})
	}
	return out
}
