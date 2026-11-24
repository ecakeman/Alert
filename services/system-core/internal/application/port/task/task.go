package task

import (
	"context"
	portshared "github.com/ecakeman/alert/services/system-core/internal/application/port/shared"
	"time"
)

type AgentTask struct {
	TaskID            string
	EpisodeID         string
	ServiceID         string
	TaskType          string
	Status            string
	Generation        int64
	Attempt           int
	MaxAttempts       int
	LeaseUntil        *time.Time
	NextRetryAt       *time.Time
	InputVersionFrom  int64
	InputVersionTo    int64
	MonitorWatermark  int64
	AppEventWatermark int64
	DeadlineAt        time.Time
	ErrorCode         string
	ErrorMessage      string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	StartedAt         *time.Time
	FinishedAt        *time.Time
}

type Ownership struct {
	ServiceID  string
	TaskID     string
	EpisodeID  string
	TaskType   string
	Generation int64
	LeaseUntil time.Time
	AcquiredAt time.Time
	UpdatedAt  time.Time
}

type TaskPatch struct {
	Status       *string
	LeaseUntil   *time.Time
	NextRetryAt  *time.Time
	Attempt      *int
	ErrorCode    *string
	ErrorMessage *string
	StartedAt    *time.Time
	FinishedAt   *time.Time
}

type ArtifactRef struct {
	ArtifactRefID   string
	EpisodeID       string
	TaskID          string
	Generation      int64
	ArtifactType    string
	ArtifactVersion int
	ContentHash     string
	ContentType     string
	StorageKey      string
	SourceRefs      []byte
	RegisteredAt    time.Time
	UpdatedAt       time.Time
}

type TaskRepo interface {
	Create(ctx context.Context, tx portshared.Tx, t AgentTask) error
	Get(ctx context.Context, tx portshared.Tx, taskID string) (AgentTask, error)
	ListActiveByEpisode(ctx context.Context, tx portshared.Tx, episodeID string) ([]AgentTask, error)
	UpdateCAS(ctx context.Context, tx portshared.Tx, taskID string, generation int64, expectedStatuses []string, patch TaskPatch) (int64, error)
	ListDueRetry(ctx context.Context, tx portshared.Tx, now time.Time, limit int) ([]AgentTask, error)
	ListDispatchPending(ctx context.Context, tx portshared.Tx, limit int) ([]AgentTask, error)
	GetForUpdate(ctx context.Context, tx portshared.Tx, taskID string) (AgentTask, error)
	ListStaleDispatched(ctx context.Context, tx portshared.Tx, updatedBefore time.Time, limit int) ([]AgentTask, error)
	ListRunningLeaseExpired(ctx context.Context, tx portshared.Tx, now time.Time, limit int) ([]AgentTask, error)
	ListByEpisode(ctx context.Context, tx portshared.Tx, episodeID string) ([]AgentTask, error)
}

type OwnershipRepo interface {
	Acquire(ctx context.Context, tx portshared.Tx, o Ownership) (bool, error)
	Get(ctx context.Context, tx portshared.Tx, serviceID string) (Ownership, error)
	Release(ctx context.Context, tx portshared.Tx, serviceID, taskID string, generation int64) (bool, error)
	ReclaimExpired(ctx context.Context, tx portshared.Tx, now time.Time) ([]string, error)
	ListExpired(ctx context.Context, tx portshared.Tx, now time.Time, limit int) ([]Ownership, error)
}

type ArtifactRepo interface {
	GetLatest(ctx context.Context, tx portshared.Tx, episodeID, artifactType string) (ArtifactRef, error)
	ListByEpisode(ctx context.Context, tx portshared.Tx, episodeID string) ([]ArtifactRef, error)
	Insert(ctx context.Context, tx portshared.Tx, a ArtifactRef) error
	GetByTypeVersion(ctx context.Context, tx portshared.Tx, episodeID, artifactType string, version int) (ArtifactRef, error)
}

// TaskDispatcher：任务提交端口，事务外调用（B7 实现）。
type TaskDispatcher interface {
	SubmitTask(ctx context.Context, task AgentTask) (accepted bool, err error)
}
