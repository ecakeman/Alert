package task

import (
	"context"
	"testing"
	"time"

	portshared "github.com/ecakeman/alert/services/system-core/internal/application/port/shared"
	porttask "github.com/ecakeman/alert/services/system-core/internal/application/port/task"
)

type reconcileTasks struct {
	*stubTasks
	task    porttask.AgentTask
	stale   []porttask.AgentTask
	expired []porttask.AgentTask
	pending []porttask.AgentTask
	due     []porttask.AgentTask
}

func (s *reconcileTasks) Get(_ context.Context, _ portshared.Tx, _ string) (porttask.AgentTask, error) {
	return s.task, nil
}

func (s *reconcileTasks) ListStaleDispatched(context.Context, portshared.Tx, time.Time, int) ([]porttask.AgentTask, error) {
	return s.stale, nil
}

func (s *reconcileTasks) ListRunningLeaseExpired(context.Context, portshared.Tx, time.Time, int) ([]porttask.AgentTask, error) {
	return s.expired, nil
}

func (s *reconcileTasks) ListDispatchPending(context.Context, portshared.Tx, int) ([]porttask.AgentTask, error) {
	return s.pending, nil
}

func (s *reconcileTasks) ListDueRetry(context.Context, portshared.Tx, time.Time, int) ([]porttask.AgentTask, error) {
	return s.due, nil
}

func (s *reconcileTasks) PromoteRetry(_ context.Context, _ portshared.Tx, taskID string, generation int64) (int64, error) {
	if s.task.TaskID != taskID || s.task.Generation != generation || s.task.Status != "RETRY_WAIT" {
		return 0, nil
	}
	s.task.Generation++
	s.task.Status = "DISPATCHED"
	return 1, nil
}

func TestReconcileSkipsTerminalAndPromotesDueRetry(t *testing.T) {
	now := time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)
	retryAt := now.Add(-time.Minute)
	dueTask := porttask.AgentTask{
		TaskID: "t1", EpisodeID: "e1", ServiceID: "s1", TaskType: "DIAGNOSIS",
		Status: "RETRY_WAIT", Generation: 1, Attempt: 1, MaxAttempts: 3, NextRetryAt: &retryAt,
	}
	terminalTask := porttask.AgentTask{
		TaskID: "t2", EpisodeID: "e1", ServiceID: "s1", TaskType: "DIAGNOSIS",
		Status: "SUCCEEDED", Generation: 1,
	}
	tasks := &reconcileTasks{
		stubTasks: &stubTasks{},
		task:      dueTask,
		stale:     []porttask.AgentTask{terminalTask},
		due:       []porttask.AgentTask{dueTask},
	}
	outbox := &createOutbox{}
	svc := &Service{deps: Deps{
		UOW: fakeUOW{}, Tasks: tasks, Outbox: outbox,
		Clock: fixedClock{now}, IDs: &createIDs{}, Lease: 30 * time.Second, RetryDelay: 30 * time.Second, MaxAttempts: 3,
	}}

	summary, err := svc.Reconcile(context.Background(), now)
	if err != nil {
		t.Fatalf("reconcile should pass: %v", err)
	}
	if summary.StaleDispatched != 0 {
		t.Fatalf("terminal task must be skipped, got stale=%d", summary.StaleDispatched)
	}
	if summary.RetryPromoted != 1 {
		t.Fatalf("due retry should be promoted once, got %d", summary.RetryPromoted)
	}
	if tasks.task.Generation != 2 {
		t.Fatalf("due retry must bump generation to 2, got %d", tasks.task.Generation)
	}
	if outbox.appends != 1 {
		t.Fatalf("promote should append one dispatch event, got %d", outbox.appends)
	}
}
