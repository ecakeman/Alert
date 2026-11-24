package episode

import (
	"time"

	"github.com/ecakeman/alert/internal/application/port"
)

type AppEventInput struct {
	ProducerID   string
	EventID      string
	EventKind    string
	HealthStatus string
	Payload      []byte
	EventTime    time.Time
}

type EpisodeView struct {
	Episode     port.Episode
	Sources     []port.SourceState
	ActiveTask  *port.AgentTask
	ServiceName string
}

type AppendEventResult struct {
	Event    port.AppEvent
	Appended bool
}
type EpisodeSummaryData struct {
	Episode            port.Episode
	ServiceName        string
	ActiveTaskStatus   string
	ActiveTaskType     string
	HasDiagnosisReport bool
}

// MonitorFieldView 是时间轴 region1 投影视图（面向传输，替代 domain.MonitorField）。
type MonitorFieldView struct {
	TargetKey        string `json:"target_key"`
	TargetType       string `json:"target_type"`
	Status           string `json:"status"`
	StatusClass      string `json:"status_class"`
	ErrorClass       string `json:"error_class"`
	StateFingerprint string `json:"state_fingerprint,omitempty"`
}
