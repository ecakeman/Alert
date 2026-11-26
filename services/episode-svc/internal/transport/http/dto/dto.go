package dto

import (
	"encoding/json"
	"time"
)

// EpisodeOut 是 episode 资源的最小出参（P4.6 再扩展 11 端点的完整视图）。
type EpisodeOut struct {
	EpisodeID             string `json:"episode_id"`
	ServiceID             string `json:"service_id"`
	Status                string `json:"status"`
	StartedBy             string `json:"started_by"`
	InputHash             string `json:"input_hash"`
	InputVersion          int64  `json:"input_version"`
	ProcessedInputVersion int64  `json:"processed_input_version"`
	FinalInputVersion     int64  `json:"final_input_version"`
	StartedAt             string `json:"started_at"`
	UpdatedAt             string `json:"updated_at"`
}

// OpenOrReuseOut 返回 episode 与是否新建。
type OpenOrReuseOut struct {
	Episode EpisodeOut `json:"episode"`
	Created bool       `json:"created"`
}

// AppEventIn 是 APP 事件接入请求体。
type AppEventIn struct {
	ProducerID   string          `json:"producer_id"`
	EventID      string          `json:"event_id"`
	EventKind    string          `json:"event_kind"`
	HealthStatus string          `json:"health_status"`
	Payload      json.RawMessage `json:"payload"`
	EventTime    string          `json:"event_time"`
}

// AppEventOut 是 APP 事件幂等结果。
type AppEventOut struct {
	EventRecordID int64  `json:"event_record_id"`
	AppEventSeq   int64  `json:"app_event_seq"`
	EventID       string `json:"event_id"`
	Appended      bool   `json:"appended"`
}

// AppendInputOut 是输入版本追加结果。
type AppendInputOut struct {
	Appended bool `json:"appended"`
}

func FormatTime(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }