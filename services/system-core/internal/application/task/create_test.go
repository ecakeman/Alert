package task

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ecakeman/alert/services/system-core/internal/application/apperr"
	portepisode "github.com/ecakeman/alert/services/system-core/internal/application/port/episode"
	portoutbox "github.com/ecakeman/alert/services/system-core/internal/application/port/outbox"
	portshared "github.com/ecakeman/alert/services/system-core/internal/application/port/shared"
	porttask "github.com/ecakeman/alert/services/system-core/internal/application/port/task"
	"github.com/ecakeman/alert/services/system-core/internal/contracts"
)

type createEpisodes struct {
	portepisode.EpisodeRepo
	episode portepisode.Episode
}

func (s *createEpisodes) Get(context.Context, portshared.Tx, string) (portepisode.Episode, error) {
	return s.episode, nil
}

func (s *createEpisodes) List(context.Context, portshared.Tx, portepisode.EpisodeFilter) ([]portepisode.Episode, error) {
	return nil, nil
}

type createInputVersions struct {
	portepisode.InputVersionRepo
	latest portepisode.InputVersion
}

func (s *createInputVersions) GetLatest(context.Context, portshared.Tx, string) (portepisode.InputVersion, error) {
	return s.latest, nil
}

type createOutbox struct {
	portoutbox.OutboxRepo
	appends int
}

func (s *createOutbox) Append(context.Context, portshared.Tx, contracts.Event, *string) error {
	s.appends++
	return nil
}

type createIDs struct{ next int }

func (s *createIDs) NewID() string {
	s.next++
	return fmt.Sprintf("id-%d", s.next)
}

type createActiveTasks struct {
	*stubTasks
	active []porttask.AgentTask
}

func (s *createActiveTasks) ListActiveByEpisode(context.Context, portshared.Tx, string) ([]porttask.AgentTask, error) {
	return s.active, nil
}

func TestCreateReusesActiveTaskAndRejectsSourceMismatch(t *testing.T) {
	now := time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)
	episode := portepisode.Episode{
		EpisodeID: "e1", ServiceID: "s1", Status: "ACTIVE",
		InputVersion: 3, ProcessedInputVersion: 1,
		MonitorWatermark: 10, AppEventWatermark: 20,
	}
	tasks := &createActiveTasks{
		stubTasks: &stubTasks{byID: map[string]porttask.AgentTask{}},
		active: []porttask.AgentTask{{
			TaskID: "t1", EpisodeID: "e1", ServiceID: "s1",
			TaskType: TaskTypeDiagnosis, Status: "DISPATCHED", Generation: 1,
		}},
	}
	outbox := &createOutbox{}
	svc := &Service{deps: Deps{
		UOW: fakeUOW{}, Tasks: tasks, Episodes: &createEpisodes{episode: episode},
		InputVersions: &createInputVersions{latest: portepisode.InputVersion{EpisodeID: "e1", InputVersion: 3}},
		Outbox:        outbox, Clock: fixedClock{now}, IDs: &createIDs{},
		MaxAttempts: 3, Deadline: time.Hour,
	}}

	view, reused, err := svc.Create(context.Background(), CreateInput{
		EpisodeID: "e1", ServiceID: "s1", TaskType: TaskTypeDiagnosis, Source: SourceEpisodeActive,
	})
	if err != nil || !reused || view.TaskID != "t1" {
		t.Fatalf("active same-type task should be reused: view=%+v reused=%v err=%v", view, reused, err)
	}
	if outbox.appends != 0 {
		t.Fatalf("reuse must not append outbox, got %d", outbox.appends)
	}

	_, _, err = svc.Create(context.Background(), CreateInput{
		EpisodeID: "e1", ServiceID: "s1", TaskType: TaskTypeArchive, Source: SourceEpisodeActive,
	})
	if apperr.Code(err) != apperr.CodeValidation {
		t.Fatalf("type/source mismatch should be validation error, got %v", err)
	}
}
