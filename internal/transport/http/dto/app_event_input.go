package dto

import "encoding/json"

// AppEventInput 是 POST /services/:id/events 的请求体。
type AppEventInput struct {
	ProducerID   string          `json:"producer_id"`
	EventID      string          `json:"event_id"`
	EventKind    string          `json:"event_kind"`
	HealthStatus string          `json:"health_status"`
	Payload      json.RawMessage `json:"payload"`
	EventTime    string          `json:"event_time"`
}
