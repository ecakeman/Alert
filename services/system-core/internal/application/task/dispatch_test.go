package task

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	portoutbox "github.com/ecakeman/alert/services/system-core/internal/application/port/outbox"
	portshared "github.com/ecakeman/alert/services/system-core/internal/application/port/shared"
	porttask "github.com/ecakeman/alert/services/system-core/internal/application/port/task"
	"github.com/ecakeman/alert/services/system-core/internal/contracts"
)

type dispatchTasks struct {
	*stubTasks
	task porttask.AgentTask
}

func (s *dispatchTasks) GetForUpdate(context.Context, portshared.Tx, string) (porttask.AgentTask, error) {
	return s.task, nil
}

func (s *dispatchTasks) UpdateCAS(_ context.Context, _ portshared.Tx, _ string, _ int64, _ []string, patch porttask.TaskPatch) (int64, error) {
	if patch.Status != nil {
		s.task.Status = *patch.Status
	}
	if patch.LeaseUntil != nil {
		s.task.LeaseUntil = patch.LeaseUntil
	}
	if patch.Attempt != nil {
		s.task.Attempt = *patch.Attempt
	}
	if patch.NextRetryAt != nil {
		s.task.NextRetryAt = patch.NextRetryAt
	}
	return 1, nil
}

type dispatchIdem struct {
	portoutbox.ConsumerIdempotencyRepo
	reserved map[string]bool
	done     map[string]bool
}

func (s *dispatchIdem) Reserve(_ context.Context, _ portshared.Tx, idem portoutbox.ConsumerIdempotency) (bool, error) {
	if s.reserved[idem.IdempotencyKey] {
		return false, nil
	}
	s.reserved[idem.IdempotencyKey] = true
	return true, nil
}

func (s *dispatchIdem) MarkDone(_ context.Context, _ portshared.Tx, _, key, _ string, _ time.Time) error {
	s.done[key] = true
	return nil
}

type stubSubmit struct {
	calls    int
	accepted bool
}

func (s *stubSubmit) SubmitTask(context.Context, porttask.AgentTask) (bool, error) {
	s.calls++
	return s.accepted, nil
}

func TestDispatchSchedulesRetryAndIsIdempotent(t *testing.T) {
	now := time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)
	task := porttask.AgentTask{
		TaskID: "t1", EpisodeID: "e1", ServiceID: "s1", TaskType: "DIAGNOSIS",
		Status: "PENDING", Generation: 1, Attempt: 0, MaxAttempts: 3,
	}
	tasks := &dispatchTasks{stubTasks: &stubTasks{}, task: task}
	ownership := &stubOwnership{rows: map[string]porttask.Ownership{}, now: now}
	idem := &dispatchIdem{reserved: map[string]bool{}, done: map[string]bool{}}
	submit := &stubSubmit{accepted: false}
	svc := &Service{deps: Deps{
		UOW: fakeUOW{}, Tasks: tasks, Ownership: ownership, Idem: idem, Submit: submit,
		Clock: fixedClock{now}, Lease: 30 * time.Second, RetryDelay: 30 * time.Second, MaxAttempts: 3,
	}}
	event := contracts.Event{
		EventID: "ev1", EventType: contracts.EventTaskDispatchRequested, SchemaVersion: 1,
		AggregateType: "task", AggregateID: "t1", OccurredAt: now,
		Payload: json.RawMessage(`{"task_id":"t1","generation":1}`),
	}

	if err := svc.HandleDispatchEvent(context.Background(), event); err != nil {
		t.Fatalf("dispatch event should pass: %v", err)
	}
	if submit.calls != 1 {
		t.Fatalf("executor should be called once, got %d", submit.calls)
	}
	if tasks.task.Status != "RETRY_WAIT" || tasks.task.Attempt != 1 {
		t.Fatalf("rejected submit should schedule retry: status=%s attempt=%d", tasks.task.Status, tasks.task.Attempt)
	}

	if err := svc.HandleDispatchEvent(context.Background(), event); err != nil {
		t.Fatalf("duplicate dispatch event should be tolerated: %v", err)
	}
	if submit.calls != 1 {
		t.Fatalf("duplicate event must not submit again, got %d", submit.calls)
	}
}
