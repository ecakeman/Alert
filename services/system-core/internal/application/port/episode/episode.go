package episode

import (
	"context"
	portshared "github.com/ecakeman/alert/services/system-core/internal/application/port/shared"
	"time"
)

type Episode struct {
	EpisodeID             string
	ServiceID             string
	Status                string
	StartedBy             string
	ResolutionPending     bool
	InputHash             string
	InputVersion          int64
	ProcessedInputVersion int64
	FinalInputVersion     int64
	MonitorWatermark      int64
	AppEventWatermark     int64
	StartedAt             time.Time
	ResolvedAt            *time.Time
	ArchivedAt            *time.Time
	UpdatedAt             time.Time
}

type InputVersion struct {
	EpisodeID         string
	InputVersion      int64
	InputHash         string
	MonitorProjection []byte
	MonitorWatermark  int64
	AppEventWatermark int64
}

type AppEvent struct {
	EventRecordID int64
	AppEventSeq   int64
	ServiceID     string
	EpisodeID     *string
	ProducerID    string
	EventID       string
	EventKind     string
	HealthStatus  string
	Payload       []byte
	ContentHash   string
	EventTime     time.Time
	ReceivedAt    time.Time
}

type EpisodeFilter struct {
	ServiceID string
	Status    string
	From      *time.Time
	To        *time.Time
	Limit     int
	Offset    int
}

type EpisodeRepo interface {
	GetActiveByService(ctx context.Context, tx portshared.Tx, serviceID string) (Episode, error)
	Get(ctx context.Context, tx portshared.Tx, episodeID string) (Episode, error)
	List(ctx context.Context, tx portshared.Tx, f EpisodeFilter) ([]Episode, error)
	Create(ctx context.Context, tx portshared.Tx, e Episode) error
	UpdateStatus(ctx context.Context, tx portshared.Tx, episodeID, status string, at time.Time, resolvedAt, archivedAt *time.Time) error
	TouchInputVersion(ctx context.Context, tx portshared.Tx, episodeID string, inputVersion int64, hash string, monitorWatermark, appWatermark int64) error
}

type InputVersionRepo interface {
	AppendIfChanged(ctx context.Context, tx portshared.Tx, v InputVersion) (bool, error)
	GetLatest(ctx context.Context, tx portshared.Tx, episodeID string) (InputVersion, error)
	ListByEpisode(ctx context.Context, tx portshared.Tx, episodeID string) ([]InputVersion, error)
}

type AppEventRepo interface {
	GetByIdentity(ctx context.Context, tx portshared.Tx, serviceID, producerID, eventID string) (AppEvent, error)
	Append(ctx context.Context, tx portshared.Tx, e AppEvent) (int64, error)
	ListByBound(ctx context.Context, tx portshared.Tx, serviceID string, fromSeq, toSeq int64, limit int) ([]AppEvent, error)
}
