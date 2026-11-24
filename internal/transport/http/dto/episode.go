package dto

import "encoding/json"

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
	PointSeq    int64  `json:"point_seq"`
	TargetID    string `json:"target_id"`
	TargetKey   string `json:"target_key"`
	TargetType  string `json:"target_type"`
	ObservedAt  string `json:"observed_at"`
	CheckStatus string `json:"check_status"`
	StatusClass string `json:"status_class"`
	ErrorClass  string `json:"error_class"`
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
	Episode              EpisodeView    `json:"episode"`
	Region1Projection    []MonitorField `json:"region1_projection"`
	MonitorStartPointSeq int64          `json:"monitor_start_point_seq"`
	MonitorPoints        []MonitorPoint `json:"monitor_points"`
	Region2Events        []AppEvent     `json:"region2_events"`
	Region3Refs          []ArtifactRef  `json:"region3_refs"`
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
	ArtifactType string         `json:"artifact_type"`
	Version      any            `json:"version"`
	ContentType  string         `json:"content_type"`
	ContentText  string         `json:"content_text"`
	ContentJSON  map[string]any `json:"content_json"`
	ContentHash  string         `json:"content_hash"`
}

// AppEventInput 是 POST /services/:id/events 的请求体。
type AppEventInput struct {
	ProducerID   string          `json:"producer_id"`
	EventID      string          `json:"event_id"`
	EventKind    string          `json:"event_kind"`
	HealthStatus string          `json:"health_status"`
	Payload      json.RawMessage `json:"payload"`
	EventTime    string          `json:"event_time"`
}
