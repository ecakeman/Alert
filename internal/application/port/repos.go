package port

import (
	"context"
	"time"

	"github.com/ecakeman/alert/internal/contracts"
)

// 13 个持久化仓储端口（实现方：infrastructure/persistence/postgres）。

type ServiceRepo interface {
	Create(ctx context.Context, tx Tx, s Service) error
	Get(ctx context.Context, tx Tx, serviceID string) (Service, error)
	GetByKey(ctx context.Context, tx Tx, serviceKey string) (Service, error)
	List(ctx context.Context, tx Tx, f ServiceFilter) ([]Service, error)

	Patch(ctx context.Context, tx Tx, serviceID string, p ServicePatch) error
	SoftDelete(ctx context.Context, tx Tx, serviceID string, at time.Time) error
}

type TargetRepo interface {
	Create(ctx context.Context, tx Tx, t MonitorTarget) error
	Get(ctx context.Context, tx Tx, targetID string) (MonitorTarget, error)
	ListByService(ctx context.Context, tx Tx, serviceID string, includeDeleted bool) ([]MonitorTarget, error)
	ListEnabled(ctx context.Context, tx Tx) ([]MonitorTarget, error)
	Patch(ctx context.Context, tx Tx, targetID string, p TargetPatch) error
	UpdateProbeResult(ctx context.Context, tx Tx, targetID, status string, failures, successes int, lastCheckAt time.Time) error
	SoftDelete(ctx context.Context, tx Tx, targetID string, at time.Time) error
}

type SourceStateRepo interface {
	Get(ctx context.Context, tx Tx, serviceID, sourceType string) (SourceState, error)
	ListByService(ctx context.Context, tx Tx, serviceID string) ([]SourceState, error)
	Upsert(ctx context.Context, tx Tx, s SourceState) error
}

type EpisodeRepo interface {
	GetActiveByService(ctx context.Context, tx Tx, serviceID string) (Episode, error)
	Get(ctx context.Context, tx Tx, episodeID string) (Episode, error)
	List(ctx context.Context, tx Tx, f EpisodeFilter) ([]Episode, error)
	Create(ctx context.Context, tx Tx, e Episode) error
	UpdateStatus(ctx context.Context, tx Tx, episodeID, status string, at time.Time, resolvedAt, archivedAt *time.Time) error
	TouchInputVersion(ctx context.Context, tx Tx, episodeID string, inputVersion int64, hash string, monitorWatermark, appWatermark int64) error
}

type InputVersionRepo interface {
	AppendIfChanged(ctx context.Context, tx Tx, v InputVersion) (bool, error)
	GetLatest(ctx context.Context, tx Tx, episodeID string) (InputVersion, error)
	ListByEpisode(ctx context.Context, tx Tx, episodeID string) ([]InputVersion, error)
}

type MonitorPointRepo interface {
	Append(ctx context.Context, tx Tx, p MonitorPoint) (int64, error)
	ListByBound(ctx context.Context, tx Tx, serviceID string, fromSeq, toSeq int64, limit int) ([]MonitorPoint, error)
	LatestSeqByService(ctx context.Context, tx Tx, serviceID string) (int64, error)
}

type AppEventRepo interface {
	GetByIdentity(ctx context.Context, tx Tx, serviceID, producerID, eventID string) (AppEvent, error)
	Append(ctx context.Context, tx Tx, e AppEvent) (int64, error)
	ListByBound(ctx context.Context, tx Tx, serviceID string, fromSeq, toSeq int64, limit int) ([]AppEvent, error)
}

type TaskRepo interface {
	Create(ctx context.Context, tx Tx, t AgentTask) error
	Get(ctx context.Context, tx Tx, taskID string) (AgentTask, error)
	ListActiveByEpisode(ctx context.Context, tx Tx, episodeID string) ([]AgentTask, error)
	UpdateCAS(ctx context.Context, tx Tx, taskID string, generation int64, expectedStatuses []string, patch TaskPatch) (int64, error)
	ListDueRetry(ctx context.Context, tx Tx, now time.Time, limit int) ([]AgentTask, error)
	ListDispatchPending(ctx context.Context, tx Tx, limit int) ([]AgentTask, error)
}

type OwnershipRepo interface {
	Acquire(ctx context.Context, tx Tx, o Ownership) (bool, error)
	Get(ctx context.Context, tx Tx, serviceID string) (Ownership, error)
	Release(ctx context.Context, tx Tx, serviceID, taskID string, generation int64) (bool, error)
	ReclaimExpired(ctx context.Context, tx Tx, now time.Time) ([]string, error)
}

type OutboxRepo interface {
	Append(ctx context.Context, tx Tx, event contracts.Event, dedupeKey *string) error
	Claim(ctx context.Context, tx Tx, owner string, until time.Time, limit int) ([]OutboxRecord, error)
	MarkPublished(ctx context.Context, tx Tx, eventID string, at time.Time) error
	MarkFailed(ctx context.Context, tx Tx, eventID string, nextRetryAt time.Time, errMsg string) error
}

type ArtifactRepo interface {
	GetLatest(ctx context.Context, tx Tx, episodeID, artifactType string) (ArtifactRef, error)
	ListByEpisode(ctx context.Context, tx Tx, episodeID string) ([]ArtifactRef, error)
	Insert(ctx context.Context, tx Tx, a ArtifactRef) error
}

type ConsumerIdempotencyRepo interface {
	Reserve(ctx context.Context, tx Tx, idem ConsumerIdempotency) (bool, error)
	MarkDone(ctx context.Context, tx Tx, consumer, key, resultCode string, at time.Time) error
}

type DashboardRepo interface {
	Summary(ctx context.Context, tx Tx, now time.Time) (DashboardSummary, error)
	Counts(ctx context.Context, tx Tx) (map[string]int, error)
	Trend(ctx context.Context, tx Tx, since time.Time) ([]TrendPoint, error)
}
