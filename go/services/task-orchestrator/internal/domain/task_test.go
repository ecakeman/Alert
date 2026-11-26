package domain

import "testing"

func TestCanAcquireNewGenerationTakesOver(t *testing.T) {
	existing := &Ownership{ServiceID: "svc-1", TaskID: "task-1", Generation: 1, LeaseUntil: 10_000}
	req := Ownership{ServiceID: "svc-1", TaskID: "task-1", Generation: 2, LeaseUntil: 10_030}
	if got := CanAcquire(existing, req, 9_000); !got.Allowed {
		t.Fatalf("new generation must take over, got %+v", got)
	}
}

func TestCanAcquireSameGenerationIsIdempotent(t *testing.T) {
	existing := &Ownership{ServiceID: "svc-1", TaskID: "task-1", Generation: 2, LeaseUntil: 10_000}
	req := Ownership{ServiceID: "svc-1", TaskID: "task-1", Generation: 2, LeaseUntil: 10_030}
	if got := CanAcquire(existing, req, 9_000); !got.Allowed {
		t.Fatalf("same generation must be idempotent, got %+v", got)
	}
}

func TestCanAcquireOtherTaskUnexpiredIsBusy(t *testing.T) {
	existing := &Ownership{ServiceID: "svc-1", TaskID: "task-1", Generation: 2, LeaseUntil: 10_000}
	req := Ownership{ServiceID: "svc-1", TaskID: "task-2", Generation: 1, LeaseUntil: 10_030}
	got := CanAcquire(existing, req, 9_000)
	if got.Allowed || got.Reason != CodeServiceBusy {
		t.Fatalf("other unexpired task must be SERVICE_BUSY, got %+v", got)
	}
}