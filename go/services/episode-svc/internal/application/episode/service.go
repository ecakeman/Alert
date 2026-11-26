package episode

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/ecakeman/alert/services/contracts"

	"github.com/ecakeman/alert/services/episode-svc/internal/application/apperr"
	portepisode "github.com/ecakeman/alert/services/episode-svc/internal/application/port/episode"
	portmonitor "github.com/ecakeman/alert/services/episode-svc/internal/application/port/monitor"
	portoutbox "github.com/ecakeman/alert/services/episode-svc/internal/application/port/outbox"
	portservice "github.com/ecakeman/alert/services/episode-svc/internal/application/port/service"
	portshared "github.com/ecakeman/alert/services/episode-svc/internal/application/port/shared"
	porttask "github.com/ecakeman/alert/services/episode-svc/internal/application/port/task"
	"github.com/ecakeman/alert/services/episode-svc/internal/domain"
)

type Deps struct {
	UOW           portshared.UnitOfWork
	Episodes      portepisode.EpisodeRepo
	InputVersions portepisode.InputVersionRepo
	AppEvents     portepisode.AppEventRepo
	Outbox        portoutbox.OutboxRepo
	Registry      portservice.Reader
	Monitor       portmonitor.RemoteReader
	Tasks         porttask.Reader
	Clock         portshared.Clock
	IDs           portshared.IDGen
}

type Service struct{ deps Deps }

func New(deps Deps) *Service { return &Service{deps: deps} }

// AppEventInput 是 APP 事件接入的用例入参（HTTP DTO 在 transport 层转换）。
type AppEventInput struct {
	ProducerID   string
	EventID      string
	EventKind    string
	HealthStatus string
	Payload      []byte
	EventTime    time.Time
}

type AppendEventResult struct {
	Event    portepisode.AppEvent
	Appended bool
}

func isAppCode(err error, code string) bool {
	var ae *apperr.Error
	return errors.As(err, &ae) && ae.Code == code
}

// OpenOrReuse：同服务有 ACTIVE 则复用，否则创建（INV-01）。
// registry 校验在事务前完成：404 → NOT_FOUND；不可达 → 503 且零写入。
func (s *Service) OpenOrReuse(ctx context.Context, serviceID string) (portepisode.Episode, bool, error) {
	if _, err := s.deps.Registry.Get(ctx, serviceID); err != nil {
		return portepisode.Episode{}, false, err
	}

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
		if err := s.appendEvent(ctx, tx, contracts.EventEpisodeStatusChanged, "episode", ep.EpisodeID,
			map[string]string{"episode_id": ep.EpisodeID, "status": "ACTIVE"}); err != nil {
			return err
		}
		return s.appendDispatchable(ctx, tx, ep, "INPUT_APPENDED", porttask.ArtifactReadiness{})
	})
	return ep, created, err
}

// AppendAppEvent：同键同内容幂等、同键不同内容 409、新键追加（INV-05）。
func (s *Service) AppendAppEvent(ctx context.Context, serviceID string, in AppEventInput) (AppendEventResult, error) {
	if in.ProducerID == "" || in.EventID == "" {
		return AppendEventResult{}, apperr.Validation("producer_id and event_id are required")
	}
	if _, err := s.deps.Registry.Get(ctx, serviceID); err != nil {
		return AppendEventResult{}, err
	}
	readiness, err := s.preDispatchReadiness(ctx, serviceID)
	if err != nil {
		return AppendEventResult{}, err
	}

	var res AppendEventResult
	err = s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
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
		if err := s.appendEvent(ctx, tx, contracts.EventEpisodeInputAppended, "service", serviceID,
			map[string]string{"service_id": serviceID, "event_id": in.EventID}); err != nil {
			return err
		}
		active, aerr := s.deps.Episodes.GetActiveByService(ctx, tx, serviceID)
		if aerr != nil {
			if isAppCode(aerr, apperr.CodeNotFound) {
				return nil
			}
			return aerr
		}
		return s.appendDispatchable(ctx, tx, active, "INPUT_APPENDED", readiness)
	})
	return res, err
}

// AppendInputVersion：仅在指纹变化时新增版本，并推进水位线（INV-04）。
func (s *Service) AppendInputVersion(ctx context.Context, episodeID string) (bool, error) {
	// 事务外先取 episode 以确定 service_id（远程点位查询必须在事务外）。
	var current portepisode.Episode
	if err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		var err error
		current, err = s.deps.Episodes.Get(ctx, tx, episodeID)
		return err
	}); err != nil {
		return false, err
	}

	readiness, err := s.deps.Tasks.GetArtifactReadiness(ctx, episodeID)
	if err != nil {
		slog.Default().WarnContext(ctx, "artifact readiness failed", "episode_id", episodeID, "reason", err.Error())
		return false, apperr.DependencyUnavailable("task unavailable")
	}
	latest, err := s.deps.Monitor.LatestSeqByService(ctx, current.ServiceID)
	if err != nil {
		slog.Default().WarnContext(ctx, "monitor latest-seq failed", "service_id", current.ServiceID, "reason", err.Error())
		return false, apperr.DependencyUnavailable("monitor unavailable")
	}
	points, err := s.deps.Monitor.ListByBound(ctx, current.ServiceID, 0, latest, 1000)
	if err != nil {
		slog.Default().WarnContext(ctx, "monitor points failed", "service_id", current.ServiceID, "reason", err.Error())
		return false, apperr.DependencyUnavailable("monitor unavailable")
	}

	var appended bool
	err = s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		ep, err := s.deps.Episodes.Get(ctx, tx, episodeID)
		if err != nil {
			return err
		}
		events, err := s.deps.AppEvents.ListByBound(ctx, tx, ep.ServiceID, 0, int64(1)<<62, 1000)
		if err != nil {
			return err
		}

		projection := buildProjection(points)
		hash, err := domain.Fingerprint(projection, buildCanon(events))
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
			MonitorProjection: projectionJSON, MonitorWatermark: latest,
			AppEventWatermark: int64(1) << 62,
		}); err != nil {
			return err
		}
		if err := s.deps.Episodes.TouchInputVersion(ctx, tx, episodeID, newVersion, hash, latest, int64(1)<<62); err != nil {
			return err
		}
		appended = true
		if err := s.appendEvent(ctx, tx, contracts.EventEpisodeInputAppended, "episode", episodeID,
			map[string]any{"episode_id": episodeID, "input_version": newVersion}); err != nil {
			return err
		}
		updated := ep
		updated.InputVersion = newVersion
		updated.InputHash = hash
		updated.MonitorWatermark = latest
		return s.appendDispatchable(ctx, tx, updated, "INPUT_APPENDED", readiness)
	})
	return appended, err
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

// ensure unused import guard for future P4.4/P4.5 additions.
var _ = fmt.Sprintf