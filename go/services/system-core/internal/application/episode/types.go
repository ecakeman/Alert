package episode

import (
	"time"

	portepisode "github.com/ecakeman/alert/services/system-core/internal/application/port/episode"
	portService "github.com/ecakeman/alert/services/system-core/internal/application/port/service"
	porttask "github.com/ecakeman/alert/services/system-core/internal/application/port/task"
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
	Episode     portepisode.Episode
	Sources     []portService.SourceState
	ActiveTask  *porttask.AgentTask
	ServiceName string
}

type AppendEventResult struct {
	Event    portepisode.AppEvent
	Appended bool
}
type EpisodeSummaryData struct {
	Episode            portepisode.Episode
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
