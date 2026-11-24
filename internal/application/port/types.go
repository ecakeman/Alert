package port

import (
	"time"

	"github.com/ecakeman/alert/internal/contracts"
)

// 领域值类型：供 application 用例与仓储端口使用；DB 行模型位于 postgres/model（S1.7 下沉）。

type Service struct {
	ServiceID              string
	ServiceKey             string
	Name                   string
	Description            string
	Enabled                bool
	MonitorAggregationMode string
	AuthorityProducerID    string
}

type MonitorTarget struct {
	TargetID             string
	ServiceID            string
	TargetKey            string
	TargetName           string
	Address              string
	TargetType           string
	Config               []byte
	IntervalMS           int
	TimeoutMS            int
	FailureThreshold     int
	RecoveryThreshold    int
	IsRequired           bool
	Enabled              bool
	Status               string
	ConsecutiveFailures  int
	ConsecutiveSuccesses int
	LastCheckAt          *time.Time
	ConfigGeneration     int64
}

type SourceState struct {
	ServiceID  string
	SourceType string
	Status     string
	Reason     string
}

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

type MonitorPoint struct {
	PointSeq         int64
	ServiceID        string
	TargetID         string
	TargetKey        string
	TargetType       string
	ObservedAt       time.Time
	ReceivedAt       time.Time
	CheckStatus      string
	StatusClass      string
	ErrorClass       string
	StateFingerprint string
	LatencyMS        *int
	ProbeGeneration  int64
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

// OutboxRecord 是 Outbox 存储行：事件信封 + 投递控制字段；不进入线上契约。
type OutboxRecord struct {
	Event           contracts.Event
	DedupeKey       *string
	PublishedAt     *time.Time
	PublishAttempts int
	NextRetryAt     *time.Time
	ClaimOwner      *string
	ClaimUntil      *time.Time
	LastError       string
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

type ConsumerIdempotency struct {
	Consumer       string
	IdempotencyKey string
	EventID        *string
	TaskID         *string
	Generation     *int64
	ResultCode     string
	ProcessedAt    *time.Time
}

// 查询过滤/更新入参。

type ServiceFilter struct {
	Query  string
	Status string
	Limit  int
	Offset int
}

type ServicePatch struct {
	Name                   *string
	Description            *string
	Enabled                *bool
	MonitorAggregationMode *string
}

type TargetPatch struct {
	TargetName        *string
	Address           *string
	Config            []byte
	IntervalMS        *int
	TimeoutMS         *int
	FailureThreshold  *int
	RecoveryThreshold *int
	IsRequired        *bool
	Enabled           *bool
}

type EpisodeFilter struct {
	ServiceID string
	Status    string
	From      *time.Time
	To        *time.Time
	Limit     int
	Offset    int
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

// Page 是分页元信息（与条目类型解耦）。
type Page struct {
	Total  int
	Limit  int
	Offset int
}
