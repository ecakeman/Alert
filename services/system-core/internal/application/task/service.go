package task

import (
	"time"

	portdispatch "github.com/ecakeman/alert/services/system-core/internal/application/port/dispatch"
	portepisode "github.com/ecakeman/alert/services/system-core/internal/application/port/episode"
	portoutbox "github.com/ecakeman/alert/services/system-core/internal/application/port/outbox"
	portshared "github.com/ecakeman/alert/services/system-core/internal/application/port/shared"
	porttask "github.com/ecakeman/alert/services/system-core/internal/application/port/task"
)

// Service 是 B7 任务链路用例编排入口。
type Service struct{ deps Deps }

func New(deps Deps) *Service { return &Service{deps: deps} }

// Deps 只含端口、时钟、ID 与配置值；不含 *http / *grpc / *pgx。
type Deps struct {
	UOW           portshared.UnitOfWork
	Tasks         porttask.TaskRepo
	Ownership     porttask.OwnershipRepo
	Artifacts     porttask.ArtifactRepo
	Episodes      portepisode.EpisodeRepo
	InputVersions portepisode.InputVersionRepo
	Outbox        portoutbox.OutboxRepo
	Idem          portoutbox.ConsumerIdempotencyRepo
	Submit        portdispatch.TaskSubmitter
	Reader        portdispatch.ArtifactContentReader
	Clock         portshared.Clock
	IDs           portshared.IDGen
	Lease         time.Duration
	MaxAttempts   int
	RetryDelay    time.Duration
	Deadline      time.Duration
}

// TaskIdentity 是 gRPC 与 HTTP 共享的任务身份。
type TaskIdentity struct {
	TaskID     string
	EpisodeID  string
	TaskType   string
	Generation int64
	LeaseToken string
}

// TaskView 是任务的对外视图。
type TaskView struct {
	TaskID           string
	EpisodeID        string
	ServiceID        string
	TaskType         string
	Status           string
	Generation       int64
	Attempt          int
	MaxAttempts      int
	LeaseUntil       *time.Time
	NextRetryAt      *time.Time
	InputVersionFrom int64
	InputVersionTo   int64
	MonitorWatermark int64
	AppWatermark     int64
	DeadlineAt       time.Time
	ErrorCode        string
	ErrorMessage     string
	CreatedAt        time.Time
	UpdatedAt        time.Time
	StartedAt        *time.Time
	FinishedAt       *time.Time
}

// EpisodeArtifactView 是路由 16 单条产物视图（元数据 + 受限正文）。
type EpisodeArtifactView struct {
	ArtifactRefID   string
	EpisodeID       string
	TaskID          string
	Generation      int64
	ArtifactType    string
	ArtifactVersion int
	ContentHash     string
	ContentType     string
	StorageKey      string
	RegisteredAt    time.Time
	Content         []byte
}

// FailureClass 是任务失败分类。
type FailureClass string

const (
	FailureRetryable FailureClass = "RETRYABLE"
	FailureFatal     FailureClass = "FATAL"
)

// DispatchResult 是派发结果。
type DispatchResult string

const (
	DispatchAccepted       DispatchResult = "ACCEPTED"
	DispatchRejected       DispatchResult = "REJECTED"
	DispatchRetryScheduled DispatchResult = "RETRY_SCHEDULED"
)

// ReconcileSummary 是恢复扫描摘要：四类修复计数与失败原因。
type ReconcileSummary struct {
	StaleDispatched    int
	LeaseExpired       int
	RetryPromoted      int
	OwnershipReclaimed int
	Failures           []string
}
