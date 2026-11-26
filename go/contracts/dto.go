package contracts

import "encoding/json"

// 本文件是 L2 域间 HTTP/JSON 载荷的唯一强类型定义。
// JSON tag 与 system-core 既有对外 DTO 同名同义；禁止用无类型映射承载结构。
// 各域服务、BFF 与 platform 的 webclient 均引用本文件，不复制结构。

// ---------- registry 域 ----------

type ServiceSummary struct {
	ServiceID       string  `json:"service_id"`
	ServiceKey      string  `json:"service_key"`
	Name            string  `json:"name"`
	Description     string  `json:"description"`
	Enabled         bool    `json:"enabled"`
	MonitorStatus   string  `json:"monitor_status"`
	AppStatus       string  `json:"app_status"`
	TargetCount     int     `json:"target_count"`
	LastCheckAt     *string `json:"last_check_at"`
	ActiveEpisodeID *string `json:"active_episode_id"`
}

type ServiceView struct {
	ServiceID              string   `json:"service_id"`
	ServiceKey             string   `json:"service_key"`
	Name                   string   `json:"name"`
	Description            string   `json:"description"`
	Enabled                bool     `json:"enabled"`
	MonitorAggregationMode string   `json:"monitor_aggregation_mode"`
	AuthorityProducerID    string   `json:"authority_producer_id"`
	MonitorStatus          string   `json:"monitor_status"`
	AppStatus              string   `json:"app_status"`
	Targets                []Target `json:"targets"`
	TargetCount            int      `json:"target_count,omitempty"`
	LastCheckAt            *string  `json:"last_check_at,omitempty"`
	ActiveEpisodeID        *string  `json:"active_episode_id,omitempty"`
}

type ServicesPage struct {
	Total    int              `json:"total"`
	Limit    int              `json:"limit"`
	Offset   int              `json:"offset"`
	Services []ServiceSummary `json:"services"`
}

type ServiceCreatePayload struct {
	ServiceKey          string        `json:"service_key"`
	Name                string        `json:"name"`
	Description         string        `json:"description"`
	AuthorityProducerID string        `json:"authority_producer_id"`
	Targets             []TargetInput `json:"targets"`
}

type ServicePatchPayload struct {
	Name                   *string `json:"name"`
	Description            *string `json:"description"`
	Enabled                *bool   `json:"enabled"`
	MonitorAggregationMode *string `json:"monitor_aggregation_mode"`
}

type Target struct {
	TargetID             string          `json:"target_id"`
	TargetKey            string          `json:"target_key"`
	TargetName           string          `json:"target_name"`
	TargetType           string          `json:"target_type"`
	Address              string          `json:"address"`
	Config               json.RawMessage `json:"config"`
	IntervalMS           int             `json:"interval_ms"`
	TimeoutMS            int             `json:"timeout_ms"`
	FailureThreshold     int             `json:"failure_threshold"`
	RecoveryThreshold    int             `json:"recovery_threshold"`
	IsRequired           bool            `json:"is_required"`
	Enabled              bool            `json:"enabled"`
	Status               string          `json:"status"`
	ConsecutiveFailures  int             `json:"consecutive_failures"`
	ConsecutiveSuccesses int             `json:"consecutive_successes"`
	LastCheckAt          *string         `json:"last_check_at"`
	ConfigGeneration     int64           `json:"config_generation"`
}

type TargetInput struct {
	TargetKey         string          `json:"target_key"`
	TargetName        string          `json:"target_name"`
	TargetType        string          `json:"target_type"`
	Address           string          `json:"address"`
	Config            json.RawMessage `json:"config"`
	IntervalMS        int             `json:"interval_ms"`
	TimeoutMS         int             `json:"timeout_ms"`
	FailureThreshold  int             `json:"failure_threshold"`
	RecoveryThreshold int             `json:"recovery_threshold"`
	IsRequired        bool            `json:"is_required"`
	Enabled           bool            `json:"enabled"`
}

type TargetPatchPayload struct {
	TargetName        *string          `json:"target_name"`
	Address           *string          `json:"address"`
	Config            *json.RawMessage `json:"config"`
	IntervalMS        *int             `json:"interval_ms"`
	TimeoutMS         *int             `json:"timeout_ms"`
	FailureThreshold  *int             `json:"failure_threshold"`
	RecoveryThreshold *int             `json:"recovery_threshold"`
	IsRequired        *bool            `json:"is_required"`
	Enabled           *bool            `json:"enabled"`
}

type SourceState struct {
	ServiceID  string `json:"service_id"`
	SourceType string `json:"source_type"`
	Status     string `json:"status"`
	Reason     string `json:"reason"`
}

// ---------- monitor 域 ----------

type MonitorPoint struct {
	PointSeq    int64  `json:"point_seq"`
	TargetID    string `json:"target_id"`
	TargetKey   string `json:"target_key"`
	TargetType  string `json:"target_type"`
	ObservedAt  string `json:"observed_at"`
	CheckStatus string `json:"check_status"`
	StatusClass string `json:"status_class"`
	ErrorClass  string `json:"error_class"`
}

type MonitorTrendPoint struct {
	BucketStart string `json:"bucket_start"`
	Success     int    `json:"success"`
	Failure     int    `json:"failure"`
}

// ---------- episode 域 ----------

type EpisodeSummary struct {
	EpisodeID             string  `json:"episode_id"`
	ServiceID             string  `json:"service_id"`
	ServiceName           string  `json:"service_name"`
	Status                string  `json:"status"`
	ResolutionPending     bool    `json:"resolution_pending"`
	StartedBy             string  `json:"started_by"`
	StartedAt             string  `json:"started_at"`
	ResolvedAt            *string `json:"resolved_at"`
	ArchivedAt            *string `json:"archived_at"`
	InputVersion          int64   `json:"input_version"`
	ProcessedInputVersion int64   `json:"processed_input_version"`
	ActiveTaskStatus      string  `json:"active_task_status"`
	ActiveTaskType        *string `json:"active_task_type,omitempty"`
	HasDiagnosisReport    *bool   `json:"has_diagnosis_report,omitempty"`
	UpdatedAt             string  `json:"updated_at"`
}

type EpisodeView struct {
	EpisodeID             string  `json:"episode_id"`
	ServiceID             string  `json:"service_id"`
	ServiceName           *string `json:"service_name,omitempty"`
	Status                string  `json:"status"`
	ResolutionPending     bool    `json:"resolution_pending"`
	StartedBy             string  `json:"started_by"`
	StartedAt             string  `json:"started_at"`
	ResolvedAt            *string `json:"resolved_at"`
	ArchivedAt            *string `json:"archived_at"`
	InputHash             string  `json:"input_hash"`
	InputVersion          int64   `json:"input_version"`
	ProcessedInputVersion int64   `json:"processed_input_version"`
	FinalInputVersion     int64   `json:"final_input_version"`
	MonitorStatus         string  `json:"monitor_status"`
	AppStatus             string  `json:"app_status"`
	ActiveTaskID          string  `json:"active_task_id"`
	ActiveTaskStatus      string  `json:"active_task_status"`
	ActiveTaskType        string  `json:"active_task_type"`
	UpdatedAt             *string `json:"updated_at,omitempty"`
}

type EpisodesPage struct {
	Total    int              `json:"total"`
	Limit    int              `json:"limit"`
	Offset   int              `json:"offset"`
	Episodes []EpisodeSummary `json:"episodes"`
}

type AppEvent struct {
	EventRecordID string          `json:"event_record_id"`
	EventID       string          `json:"event_id"`
	EventKind     string          `json:"event_kind"`
	HealthStatus  string          `json:"health_status"`
	EventTime     string          `json:"event_time"`
	ReceivedAt    string          `json:"received_at"`
	Payload       json.RawMessage `json:"payload"`
}

type AppEventInput struct {
	ProducerID   string          `json:"producer_id"`
	EventID      string          `json:"event_id"`
	EventKind    string          `json:"event_kind"`
	HealthStatus string          `json:"health_status"`
	Payload      json.RawMessage `json:"payload"`
	EventTime    string          `json:"event_time"`
}

// ArtifactReadiness 是派发快照中的产物就绪度。
type ArtifactReadiness struct {
	Investigation   bool `json:"investigation"`
	DiagnosisReport bool `json:"diagnosis_report"`
	HashesMatch     bool `json:"hashes_match"`
}

// DispatchSnapshot 是 episode.dispatchable.v1 的载荷，task 侧零回查建任务。
type DispatchSnapshot struct {
	EpisodeID             string            `json:"episode_id"`
	ServiceID             string            `json:"service_id"`
	EpisodeStatus         string            `json:"episode_status"`
	InputVersionFrom      int64             `json:"input_version_from"`
	InputVersionTo        int64             `json:"input_version_to"`
	MonitorWatermark      int64             `json:"monitor_watermark"`
	AppEventWatermark     int64             `json:"app_event_watermark"`
	FinalInputVersion     int64             `json:"final_input_version"`
	ProcessedInputVersion int64             `json:"processed_input_version"`
	Reason                string            `json:"reason"`
	ArtifactReadiness     ArtifactReadiness `json:"artifact_readiness"`
}

// ---------- task 域 ----------

// ActiveTask 用于 episode 解除判定的"无活动任务"条件。
type ActiveTask struct {
	TaskID     string `json:"task_id"`
	EpisodeID  string `json:"episode_id"`
	TaskType   string `json:"task_type"`
	Status     string `json:"status"`
	Generation int64  `json:"generation"`
}

// TaskView 是任务对外视图（L2 与人工重试响应共用）。
type TaskView struct {
	TaskID           string  `json:"task_id"`
	EpisodeID        string  `json:"episode_id"`
	ServiceID        string  `json:"service_id"`
	TaskType         string  `json:"task_type"`
	Status           string  `json:"status"`
	Generation       int64   `json:"generation"`
	Attempt          int     `json:"attempt"`
	MaxAttempts      int     `json:"max_attempts"`
	LeaseUntil       *string `json:"lease_until"`
	NextRetryAt      *string `json:"next_retry_at"`
	InputVersionFrom int64   `json:"input_version_from"`
	InputVersionTo   int64   `json:"input_version_to"`
	MonitorWatermark int64   `json:"monitor_watermark"`
	AppWatermark     int64   `json:"app_watermark"`
	DeadlineAt       string  `json:"deadline_at"`
	ErrorCode        string  `json:"error_code"`
	ErrorMessage     string  `json:"error_message"`
	CreatedAt        string  `json:"created_at"`
	UpdatedAt        string  `json:"updated_at"`
	StartedAt        *string `json:"started_at"`
	FinishedAt       *string `json:"finished_at"`
}

type Artifact struct {
	ArtifactType    string `json:"artifact_type"`
	ArtifactVersion int    `json:"artifact_version"`
	StorageKey      string `json:"storage_key"`
	ContentType     string `json:"content_type"`
	ContentHash     string `json:"content_hash"`
	UpdatedAt       string `json:"updated_at"`
}

type ArtifactList struct {
	Artifacts []Artifact `json:"artifacts"`
}

// ---------- 聚合视图 ----------

// Stats 是各域 /internal/stats 的通用计数结构。
type Stats struct {
	Window         string              `json:"window"`
	GeneratedAt    string              `json:"generated_at"`
	Services       map[string]int      `json:"services,omitempty"`
	Targets        map[string]int      `json:"targets,omitempty"`
	Episodes       map[string]int      `json:"episodes,omitempty"`
	RecentEpisodes []EpisodeSummary    `json:"recent_episodes,omitempty"`
	MonitorTrend   []MonitorTrendPoint `json:"monitor_trend,omitempty"`
}
