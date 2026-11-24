package contracts

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Event 是事件线上信封的唯一 Go 表示；公开字段见 contracts/events/README.md。
// 投递控制字段（claim/ack/重试）不在此结构中，仅存在于 Outbox 存储行。
type Event struct {
	EventID       string          `json:"event_id"`
	EventType     string          `json:"event_type"`
	SchemaVersion int             `json:"schema_version"`
	AggregateType string          `json:"aggregate_type"`
	AggregateID   string          `json:"aggregate_id"`
	OccurredAt    time.Time       `json:"occurred_at"`
	TraceID       string          `json:"trace_id,omitempty"`
	Payload       json.RawMessage `json:"payload"`
}

// 已支持的事件类型（schema_version=1）。
const (
	EventServiceStateChanged   = "service.state.changed.v1"
	EventEpisodeStatusChanged  = "episode.status.changed.v1"
	EventEpisodeInputAppended  = "episode.input.appended.v1"
	EventTaskDispatchRequested = "task.dispatch.requested.v1"
)

// ErrUnsupportedEvent 表示事件类型或版本不受支持。
var ErrUnsupportedEvent = errors.New("unsupported event type or schema version")

// Supported 判断事件类型与版本是否受支持。
func Supported(eventType string, schemaVersion int) bool {
	switch eventType {
	case EventServiceStateChanged, EventEpisodeStatusChanged, EventEpisodeInputAppended, EventTaskDispatchRequested:
		return schemaVersion == 1
	default:
		return false
	}
}

// Validate 校验信封必填字段、payload 与版本支持。
func (e Event) Validate() error {
	if e.EventID == "" || e.EventType == "" || e.AggregateType == "" || e.AggregateID == "" {
		return fmt.Errorf("%w: missing identity fields", ErrUnsupportedEvent)
	}
	if e.SchemaVersion < 1 {
		return fmt.Errorf("%w: schema_version must be >= 1", ErrUnsupportedEvent)
	}
	if len(e.Payload) == 0 {
		return fmt.Errorf("%w: empty payload", ErrUnsupportedEvent)
	}
	if !Supported(e.EventType, e.SchemaVersion) {
		return fmt.Errorf("%w: type=%s version=%d", ErrUnsupportedEvent, e.EventType, e.SchemaVersion)
	}
	if e.OccurredAt.IsZero() {
		return fmt.Errorf("%w: occurred_at is zero", ErrUnsupportedEvent)
	}
	return nil
}
