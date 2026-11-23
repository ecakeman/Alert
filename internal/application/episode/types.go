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
	Episode    port.Episode
	Sources    []port.SourceState
	ActiveTask *port.AgentTask
}

type AppendEventResult struct {
	Event    port.AppEvent
	Appended bool
}