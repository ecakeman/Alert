package dto

import "encoding/json"

// 与 apps/console/src/api/types.ts 一一对齐，JSON 使用 snake_case。

type DashboardSummary struct {
	GeneratedAt    string         `json:"generated_at"`
	Services       map[string]int `json:"services"`
	Targets        map[string]int `json:"targets"`
	Episodes       map[string]int `json:"episodes"`
	RecentEpisodes []EpisodeSummary `json:"recent_episodes"`
	MonitorTrend   []MonitorTrendPoint `json:"monitor_trend"`
}

type MonitorTrendPoint struct {
	BucketStart string `json:"bucket_start"`
	Success     int    `json:"success"`
	Failure     int    `json:"failure"`
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

type ServiceSummary struct {
	ServiceID        string  `json:"service_id"`
	ServiceKey       string  `json:"service_key"`
	Name             string  `json:"name"`
	Description      string  `json:"description"`
	Enabled          bool    `json:"enabled"`
	MonitorStatus    string  `json:"monitor_status"`
	AppStatus        string  `json:"app_status"`
	TargetCount      int     `json:"target_count"`
	LastCheckAt      *string `json:"last_check_at"`
	ActiveEpisodeID  *string `json:"active_episode_id"`
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
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Enabled     *bool   `json:"enabled"`
}

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

type MonitorField struct {
	TargetKey        string `json:"target_key"`
	TargetType       string `json:"target_type"`
	Status           string `json:"status"`
	StatusClass      string `json:"status_class"`
	ErrorClass       string `json:"error_class"`
	StateFingerprint string `json:"state_fingerprint,omitempty"`
}

type MonitorPoint struct {
	PointSeq     int64  `json:"point_seq"`
	TargetID     string `json:"target_id"`
	TargetKey    string `json:"target_key"`
	TargetType   string `json:"target_type"`
	ObservedAt   string `json:"observed_at"`
	CheckStatus  string `json:"check_status"`
	StatusClass  string `json:"status_class"`
	ErrorClass   string `json:"error_class"`
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

type ArtifactRef struct {
	ArtifactType    string `json:"artifact_type"`
	ArtifactVersion int    `json:"artifact_version"`
	StorageKey      string `json:"storage_key"`
	ContentType     string `json:"content_type"`
	ContentHash     string `json:"content_hash"`
	UpdatedAt       string `json:"updated_at"`
}

type EpisodeTimeline struct {
	Episode             EpisodeView     `json:"episode"`
	Region1Projection   []MonitorField  `json:"region1_projection"`
	MonitorStartPointSeq int64          `json:"monitor_start_point_seq"`
	MonitorPoints       []MonitorPoint  `json:"monitor_points"`
	Region2Events       []AppEvent      `json:"region2_events"`
	Region3Refs         []ArtifactRef   `json:"region3_refs"`
}

type ServiceTimelineRecord struct {
	RecordType string          `json:"record_type"`
	RecordID   string          `json:"record_id"`
	EventTime  string          `json:"event_time"`
	ReceivedAt string          `json:"received_at"`
	Source     string          `json:"source"`
	Content    json.RawMessage `json:"content"`
}

type ServiceTimeline struct {
	Records []ServiceTimelineRecord `json:"records"`
}

type EpisodeArtifact struct {
	ArtifactType string                 `json:"artifact_type"`
	Version      any                    `json:"version"`
	ContentType  string                 `json:"content_type"`
	ContentText  string                 `json:"content_text"`
	ContentJSON  map[string]any         `json:"content_json"`
	ContentHash  string                 `json:"content_hash"`
}