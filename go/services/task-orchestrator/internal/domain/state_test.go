package domain

import "testing"

// INV-10：故障必须先解除（RESOLVED）再归档（ARCHIVED），ACTIVE 不得直接归档。
func TestINV10_IncidentTransitions(t *testing.T) {
	legal := [][2]IncidentStatus{
		{IncidentActive, IncidentResolved},
		{IncidentResolved, IncidentArchived},
	}
	for _, c := range legal {
		if err := CanTransitionIncident(c[0], c[1]); err != nil {
			t.Fatalf("expected legal %v->%v, got %v", c[0], c[1], err)
		}
	}
	illegal := [][2]IncidentStatus{
		{IncidentActive, IncidentArchived},
		{IncidentResolved, IncidentActive},
		{IncidentArchived, IncidentActive},
		{IncidentArchived, IncidentResolved},
	}
	for _, c := range illegal {
		if err := CanTransitionIncident(c[0], c[1]); !IsCode(err, CodeInvalidTransition) {
			t.Fatalf("expected INVALID_TRANSITION for %v->%v, got %v", c[0], c[1], err)
		}
	}
}

// INV-12：SUCCEEDED / FAILED 为终态，不得自动回流。
func TestINV12_TaskTerminalTransitions(t *testing.T) {
	legal := [][2]TaskStatus{
		{TaskPending, TaskDispatched},
		{TaskDispatched, TaskRunning},
		{TaskRunning, TaskSucceeded},
		{TaskRunning, TaskRetryWait},
		{TaskRunning, TaskFailed},
		{TaskRetryWait, TaskDispatched},
	}
	for _, c := range legal {
		if err := CanTransitionTask(c[0], c[1]); err != nil {
			t.Fatalf("expected legal %v->%v, got %v", c[0], c[1], err)
		}
	}
	illegal := [][2]TaskStatus{
		{TaskSucceeded, TaskRunning},
		{TaskSucceeded, TaskPending},
		{TaskFailed, TaskDispatched},
		{TaskFailed, TaskRetryWait},
		{TaskPending, TaskSucceeded},
	}
	for _, c := range illegal {
		if err := CanTransitionTask(c[0], c[1]); !IsCode(err, CodeInvalidTransition) {
			t.Fatalf("expected reject %v->%v, got %v", c[0], c[1], err)
		}
	}
}