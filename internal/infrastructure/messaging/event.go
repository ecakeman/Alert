package messaging

import (
	"encoding/json"
	"time"
)

// Event 是发布到 Kafka 的领域事件信封。
type Event struct {
	EventID       string          `json:"event_id"`
	EventType     string          `json:"event_type"`
	SchemaVersion int             `json:"schema_version"`
	AggregateType string          `json:"aggregate_type"`
	AggregateID   string          `json:"aggregate_id"`
	OccurredAt    time.Time       `json:"occurred_at"`
	TraceID       string          `json:"trace_id,omitempty"`
	Payload       json.RawMessage `json:"payload,omitempty"`
}

func Encode(e Event) ([]byte, error) { return json.Marshal(e) }

func Decode(body []byte) (Event, error) {
	var e Event
	err := json.Unmarshal(body, &e)
	return e, err
}
