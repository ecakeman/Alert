package task

import (
	"context"
	"testing"
	"time"

	"github.com/ecakeman/alert/services/system-core/internal/application/apperr"
	portshared "github.com/ecakeman/alert/services/system-core/internal/application/port/shared"
	porttask "github.com/ecakeman/alert/services/system-core/internal/application/port/task"
	"github.com/ecakeman/alert/services/system-core/internal/domain"
)

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

type fakeUOW struct{}

func (fakeUOW) WithinTx(ctx context.Context, fn func(context.Context, portshared.Tx) error) error {
	return fn(ctx, nil)
}

type stubTasks struct{ byID map[string]porttask.AgentTask }

func (s *stubTasks) Create(context.Context, portshared.Tx, porttask.AgentTask) error { return nil }

func (s *stubTasks) PromoteRetry(context.Context, portshared.Tx, string, int64) (int64, error) {
	return 0, nil
}

func (s *stubTasks) Get(_ context.Context, _ portshared.Tx, taskID string) (porttask.AgentTask, error) {
	t, ok := s.byID[taskID]
	if !ok {
		return porttask.AgentTask{}, apperr.NotFound("task not found")
	}
	return t, nil
}

func (s *stubTasks) GetForUpdate(ctx context.Context, tx portshared.Tx, taskID string) (porttask.AgentTask, error) {
	return s.Get(ctx, tx, taskID)
}

func (s *stubTasks) ListActiveByEpisode(context.Context, portshared.Tx, string) ([]porttask.AgentTask, error) {
	return nil, nil
}

func (s *stubTasks) UpdateCAS(context.Context, portshared.Tx, string, int64, []string, porttask.TaskPatch) (int64, error) {
	return 0, nil
}

func (s *stubTasks) ListDueRetry(context.Context, portshared.Tx, time.Time, int) ([]porttask.AgentTask, error) {
	return nil, nil
}

func (s *stubTasks) ListDispatchPending(context.Context, portshared.Tx, int) ([]porttask.AgentTask, error) {
	return nil, nil
}

func (s *stubTasks) ListStaleDispatched(context.Context, portshared.Tx, time.Time, int) ([]porttask.AgentTask, error) {
	return nil, nil
}

func (s *stubTasks) ListRunningLeaseExpired(context.Context, portshared.Tx, time.Time, int) ([]porttask.AgentTask, error) {
	return nil, nil
}

func (s *stubTasks) ListByEpisode(context.Context, portshared.Tx, string) ([]porttask.AgentTask, error) {
	return nil, nil
}

type stubOwnership struct {
	rows map[string]porttask.Ownership
	now  time.Time
}

func (s *stubOwnership) Acquire(_ context.Context, _ portshared.Tx, o porttask.Ownership) (bool, error) {
	if cur, ok := s.rows[o.ServiceID]; ok {
		sameTask := cur.TaskID == o.TaskID && cur.Generation == o.Generation
		if !sameTask && cur.LeaseUntil.After(s.now) {
			return false, nil
		}
	}
	s.rows[o.ServiceID] = o
	return true, nil
}

func (s *stubOwnership) Get(_ context.Context, _ portshared.Tx, serviceID string) (porttask.Ownership, error) {
	o, ok := s.rows[serviceID]
	if !ok {
		return porttask.Ownership{}, apperr.NotFound("ownership not found")
	}
	return o, nil
}

func (s *stubOwnership) Release(_ context.Context, _ portshared.Tx, serviceID, taskID string, generation int64) (bool, error) {
	o, ok := s.rows[serviceID]
	if !ok || o.TaskID != taskID || o.Generation != generation {
		return false, nil
	}
	delete(s.rows, serviceID)
	return true, nil
}

func (s *stubOwnership) ReclaimExpired(context.Context, portshared.Tx, time.Time) ([]string, error) {
	return nil, nil
}

func (s *stubOwnership) ListExpired(context.Context, portshared.Tx, time.Time, int) ([]porttask.Ownership, error) {
	return nil, nil
}

func TestOwnershipAcquireReleaseAndIdempotency(t *testing.T) {
	now := time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)
	tasks := &stubTasks{byID: map[string]porttask.AgentTask{
		"t1": {TaskID: "t1", ServiceID: "s1", EpisodeID: "e1", TaskType: "DIAGNOSIS", Generation: 1},
		"t2": {TaskID: "t2", ServiceID: "s1", EpisodeID: "e2", TaskType: "DIAGNOSIS", Generation: 1},
		"t3": {TaskID: "t3", ServiceID: "s2", EpisodeID: "e3", TaskType: "DIAGNOSIS", Generation: 1},
	}}
	own := &stubOwnership{rows: map[string]porttask.Ownership{}, now: now}
	svc := &Service{deps: Deps{UOW: fakeUOW{}, Tasks: tasks, Ownership: own, Clock: fixedClock{now}, Lease: 30 * time.Second}}

	ok, err := svc.Acquire(context.Background(), "t1", 1, 30*time.Second)
	if err != nil || !ok {
		t.Fatalf("first acquire should pass: ok=%v err=%v", ok, err)
	}

	_, err = svc.Acquire(context.Background(), "t2", 1, 30*time.Second)
	if apperr.Code(err) != domain.CodeServiceBusy {
		t.Fatalf("same service must be busy, got %v", err)
	}

	ok, err = svc.Acquire(context.Background(), "t3", 1, 30*time.Second)
	if err != nil || !ok {
		t.Fatalf("cross service should pass: ok=%v err=%v", ok, err)
	}

	ok, err = svc.Acquire(context.Background(), "t1", 1, 30*time.Second)
	if err != nil || !ok {
		t.Fatalf("same task+generation should be idempotent: ok=%v err=%v", ok, err)
	}
}
