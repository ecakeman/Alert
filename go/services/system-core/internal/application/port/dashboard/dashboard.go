package dashboard

import (
	"context"
	portshared "github.com/ecakeman/alert/services/system-core/internal/application/port/shared"
	"time"
)

type DashboardSummary struct {
	GeneratedAt time.Time
	Services    map[string]int
	Targets     map[string]int
	Episodes    map[string]int
	Recent      []RecentEpisode
	Trend       []TrendPoint
}

// RecentEpisode 是看板"最近故障"读模型（对齐 dto.EpisodeSummary 形状）。
type RecentEpisode struct {
	EpisodeID             string
	ServiceID             string
	ServiceName           string
	Status                string
	ResolutionPending     bool
	StartedBy             string
	StartedAt             time.Time
	ResolvedAt            *time.Time
	ArchivedAt            *time.Time
	InputVersion          int64
	ProcessedInputVersion int64
	ActiveTaskStatus      string
	ActiveTaskType        string
	HasDiagnosisReport    bool
	UpdatedAt             time.Time
}

type TrendPoint struct {
	Bucket  time.Time
	Success int
	Failure int
}

type DashboardRepo interface {
	Summary(ctx context.Context, tx portshared.Tx, now time.Time, window time.Duration) (DashboardSummary, error)
}

// EpisodeSource 是 dashboard 的 episode 域数据源（P4.8：单体不再直读 episode schema）。
type EpisodeSource interface {
	Counts(ctx context.Context) (map[string]int, error)
	Recent(ctx context.Context, limit int) ([]RecentEpisode, error)
}

// RecentEnricher 用单体自有表补齐 episode 域之外的展示字段。
type RecentEnricher interface {
	EnrichRecent(ctx context.Context, tx portshared.Tx, items []RecentEpisode) ([]RecentEpisode, error)
}
