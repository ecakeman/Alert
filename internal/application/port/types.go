package port

import "time"

// 领域值类型：字段与 system schema 列一一对应，供 application 用例与仓储端口使用。

type Service struct {
	ServiceID              string
	ServiceKey             string
	Name                   string
	Description            string
	Enabled                bool
	MonitorAggregationMode string
	AuthorityProducerID    string
	DeletedAt              *time.Time
	CreatedAt              time.Time
	UpdatedAt              time.Time
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
	DeletedAt            *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

type SourceState struct {
	ServiceID    string
	SourceType   string
	Status       string
	Reason       string
	UpdatedAt    time.Time
	LastChangeAt time.Time
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
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

type InputVersion struct {
	EpisodeID         string
	InputVersion      int64
	InputHash         string
	MonitorProjection []byte
	MonitorWatermark  int64
	AppEventWatermark int64
	CreatedAt         time.Time
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

type OutboxEvent struct {
	EventID         string
	EventType       string
	SchemaVersion   int
	AggregateType   string
	AggregateID     string
	Payload         []byte
	DedupeKey       *string
	CreatedAt       time.Time
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
	GeneratedAt    time.Time
	Services       map[string]int
	Targets        map[string]int
	Episodes       map[string]int
	RecentEpisodes []Episode
}

type TrendPoint struct {
	Bucket time.Time
	Count  int
}
