package task

import (
	"context"
	"testing"
	"time"

	"github.com/ecakeman/alert/services/system-core/internal/application/apperr"
	portepisode "github.com/ecakeman/alert/services/system-core/internal/application/port/episode"
	portshared "github.com/ecakeman/alert/services/system-core/internal/application/port/shared"
	porttask "github.com/ecakeman/alert/services/system-core/internal/application/port/task"
	"github.com/ecakeman/alert/services/system-core/internal/domain"
)

type retryTasks struct {
	*stubTasks
	recent  []porttask.AgentTask
	active  []porttask.AgentTask
	created []porttask.AgentTask
}

func (s *retryTasks) ListByEpisode(context.Context, portshared.Tx, string) ([]porttask.AgentTask, error) {
	return s.recent, nil
}

func (s *retryTasks) ListActiveByEpisode(context.Context, portshared.Tx, string) ([]porttask.AgentTask, error) {
	return s.active, nil
}

func (s *retryTasks) Create(_ context.Context, _ portshared.Tx, task porttask.AgentTask) error {
	s.created = append(s.created, task)
	return nil
}

func TestRetryRejectsNonTerminalAndBumpsGeneration(t *testing.T) {
	now := time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)
	episode := portepisode.Episode{
		EpisodeID: "e1", ServiceID: "s1", Status: "ACTIVE",
		InputVersion: 3, ProcessedInputVersion: 1,
	}
	tasks := &retryTasks{stubTasks: &stubTasks{}, recent: []porttask.AgentTask{{
		TaskID: "t1", EpisodeID: "e1", ServiceID: "s1", TaskType: TaskTypeDiagnosis,
		Status: "RUNNING", Generation: 1,
	}}}
	svc := &Service{deps: Deps{
		UOW: fakeUOW{}, Tasks: tasks,
		Episodes: &createEpisodes{episode: episode},
		Outbox:   &createOutbox{}, Clock: fixedClock{now}, IDs: &createIDs{},
		MaxAttempts: 3, Deadline: time.Hour,
	}}

	if _, err := svc.RetryDiagnosis(context.Background(), "e1"); apperr.Code(err) != domain.CodeStateConflict {
		t.Fatalf("non-terminal task should be rejected, got %v", err)
	}

	tasks.recent[0].Status = "SUCCEEDED"
	view, err := svc.RetryDiagnosis(context.Background(), "e1")
	if err != nil {
		t.Fatalf("terminal task retry should pass: %v", err)
	}
	if view.Generation != 2 || view.TaskType != TaskTypeDiagnosis || len(tasks.created) != 1 {
		t.Fatalf("retry should create generation 2: view=%+v created=%d", view, len(tasks.created))
	}
}
