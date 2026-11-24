package task

import (
	"context"
	"testing"
	"time"

	portshared "github.com/ecakeman/alert/services/system-core/internal/application/port/shared"
	porttask "github.com/ecakeman/alert/services/system-core/internal/application/port/task"
)

type progressTasks struct {
	*stubTasks
	task porttask.AgentTask
}

func (s *progressTasks) GetForUpdate(context.Context, portshared.Tx, string) (porttask.AgentTask, error) {
	return s.task, nil
}

func (s *progressTasks) UpdateCAS(_ context.Context, _ portshared.Tx, _ string, _ int64, _ []string, patch porttask.TaskPatch) (int64, error) {
	if patch.Status != nil {
		s.task.Status = *patch.Status
	}
	if patch.Attempt != nil {
		s.task.Attempt = *patch.Attempt
	}
	if patch.FinishedAt != nil {
		s.task.FinishedAt = patch.FinishedAt
	}
	return 1, nil
}

type progressEpisodes struct {
	*createEpisodes
	target   int64
	advanced bool
}

func (s *progressEpisodes) AdvanceProcessedVersion(_ context.Context, _ portshared.Tx, _ string, target int64) (int64, error) {
	s.target = target
	s.advanced = true
	return 1, nil
}

func identityFor(task porttask.AgentTask, leaseUntil time.Time) TaskIdentity {
	return TaskIdentity{
		TaskID:     task.TaskID,
		EpisodeID:  task.EpisodeID,
		TaskType:   task.TaskType,
		Generation: task.Generation,
		LeaseToken: encodeLeaseToken(task.TaskID, task.Generation, leaseUntil.Unix()),
	}
}

func TestProgressCompleteAndFailure(t *testing.T) {
	now := time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)
	leaseUntil := now.Add(2 * time.Minute)
	base := porttask.AgentTask{
		TaskID: "t1", EpisodeID: "e1", ServiceID: "s1", TaskType: "DIAGNOSIS",
		Status: "RUNNING", Generation: 1, Attempt: 0, MaxAttempts: 3,
		LeaseUntil: &leaseUntil, InputVersionTo: 5,
	}
	tasks := &progressTasks{stubTasks: &stubTasks{}, task: base}
	episodes := &progressEpisodes{createEpisodes: &createEpisodes{}}
	ownership := &stubOwnership{rows: map[string]porttask.Ownership{}, now: now}
	svc := &Service{deps: Deps{
		UOW: fakeUOW{}, Tasks: tasks, Episodes: episodes, Ownership: ownership,
		Clock: fixedClock{now}, Lease: 30 * time.Second, RetryDelay: 30 * time.Second, MaxAttempts: 3,
	}}
	id := identityFor(base, leaseUntil)

	ownership.rows["s1"] = porttask.Ownership{ServiceID: "s1", TaskID: "t1", Generation: 1, LeaseUntil: leaseUntil}
	if err := svc.Complete(context.Background(), id); err != nil {
		t.Fatalf("complete should pass: %v", err)
	}
	if tasks.task.Status != "SUCCEEDED" || !episodes.advanced || episodes.target != 5 {
		t.Fatalf("complete effects mismatch: status=%s advanced=%v target=%d", tasks.task.Status, episodes.advanced, episodes.target)
	}
	if _, ok := ownership.rows["s1"]; ok {
		t.Fatal("complete should release ownership")
	}

	tasks.task = base
	ownership.rows["s1"] = porttask.Ownership{ServiceID: "s1", TaskID: "t1", Generation: 1, LeaseUntil: leaseUntil}
	if err := svc.ReportFailure(context.Background(), id, FailureRetryable, "E_RETRY", "retry"); err != nil {
		t.Fatalf("retryable failure should pass: %v", err)
	}
	if tasks.task.Status != "RETRY_WAIT" || tasks.task.Attempt != 1 {
		t.Fatalf("retryable should become RETRY_WAIT: status=%s attempt=%d", tasks.task.Status, tasks.task.Attempt)
	}

	tasks.task = base
	ownership.rows["s1"] = porttask.Ownership{ServiceID: "s1", TaskID: "t1", Generation: 1, LeaseUntil: leaseUntil}
	if err := svc.ReportFailure(context.Background(), id, FailureFatal, "E_FATAL", "fatal"); err != nil {
		t.Fatalf("fatal failure should pass: %v", err)
	}
	if tasks.task.Status != "FAILED" {
		t.Fatalf("fatal should become FAILED, got %s", tasks.task.Status)
	}
	if _, ok := ownership.rows["s1"]; ok {
		t.Fatal("fatal failure should release ownership")
	}
}
