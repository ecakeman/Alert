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

	// P0.16 预演：monitor 域自有事件（拆分后发布到 monitor topic）。
	EventMonitorTargetStateChanged  = "monitor.target.state.changed.v1"
	EventMonitorServiceStateChanged = "monitor.service.state.changed.v1"

	// P0.18 预演：episode 派发决策快照（拆分后发布到 episode topic）。
	EventEpisodeDispatchable = "episode.dispatchable.v1"
)

// ErrUnsupportedEvent 表示事件类型或版本不受支持。
var ErrUnsupportedEvent = errors.New("unsupported event type or schema version")

// ArtifactReadiness 是派发快照中的产物就绪度。
type ArtifactReadiness struct {
	Investigation   bool `json:"investigation"`
	DiagnosisReport bool `json:"diagnosis_report"`
	HashesMatch     bool `json:"hashes_match"`
}

// DispatchSnapshot 是 episode.dispatchable.v1 的载荷；task 侧零回查建任务。
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

// Validate 校验快照必填字段与输入区间；缺失即拒绝建任务。
func (s DispatchSnapshot) Validate() error {
	if s.EpisodeID == "" || s.ServiceID == "" || s.EpisodeStatus == "" || s.Reason == "" {
		return errors.New("dispatch snapshot missing identity fields")
	}
	if s.InputVersionFrom < 1 || s.InputVersionTo < 1 || s.InputVersionFrom > s.InputVersionTo {
		return errors.New("dispatch snapshot invalid input version range")
	}
	if s.Reason != "INPUT_APPENDED" && s.Reason != "EPISODE_RESOLVED" {
		return errors.New("dispatch snapshot invalid reason")
	}
	return nil
}

// Supported 判断事件类型与版本是否受支持。
func Supported(eventType string, schemaVersion int) bool {
	switch eventType {
	case EventServiceStateChanged, EventEpisodeStatusChanged, EventEpisodeInputAppended,
		EventTaskDispatchRequested, EventMonitorTargetStateChanged, EventMonitorServiceStateChanged,
		EventEpisodeDispatchable:
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
