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
