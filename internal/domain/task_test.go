package domain

import (
	"reflect"
	"strings"
	"testing"
)

// INV-07：同 Service 串行、跨 Service 并发、过期可领、同代次幂等。
func TestINV07_ServiceSerialization(t *testing.T) {
	now := int64(1000)
	existing := &Ownership{ServiceID: "s1", TaskID: "t1", Generation: 1, LeaseUntil: 2000}

	if d := CanAcquire(existing, Ownership{ServiceID: "s1", TaskID: "t2", Generation: 1}, now); d.Allowed || d.Reason != CodeServiceBusy {
		t.Fatalf("same service other task should be busy: %+v", d)
	}
	if d := CanAcquire(existing, Ownership{ServiceID: "s2", TaskID: "t9"}, now); !d.Allowed {
		t.Fatalf("different service should allow: %+v", d)
	}
	if d := CanAcquire(existing, Ownership{ServiceID: "s1", TaskID: "t2"}, 3000); !d.Allowed {
		t.Fatalf("expired lease should allow: %+v", d)
	}
	if d := CanAcquire(existing, Ownership{ServiceID: "s1", TaskID: "t1", Generation: 1}, now); !d.Allowed {
		t.Fatalf("same task+generation should be idempotent: %+v", d)
	}
}

// INV-08：旧代次/失租/终态/类型不符/跨故障/不存在 一律拒绝，拒绝码稳定。
func TestINV08_Fence(t *testing.T) {
	base := FenceInput{
		Found: true, IncidentID: "inc-1", ExpectedIncident: "inc-1",
		TaskType: "DIAGNOSIS", ExpectedTaskType: "DIAGNOSIS",
		Generation: 2, CurrentGeneration: 2, Status: TaskRunning, LeaseValid: true,
	}
	if d := Fence(base); !d.Accepted {
		t.Fatalf("expected accept, got %+v", d)
	}

	check := func(name string, in FenceInput, want string) {
		t.Helper()
		if d := Fence(in); d.Accepted || d.Code != want {
			t.Fatalf("%s: expected reject %s, got %+v", name, want, d)
		}
	}
	stale := base
	stale.Generation = 1
	check("stale generation", stale, CodeStaleGeneration)

	noLease := base
	noLease.LeaseValid = false
	check("lease lost", noLease, CodeLeaseLost)

	wrongType := base
	wrongType.TaskType = "ARCHIVE"
	check("type mismatch", wrongType, CodeTypeMismatch)

	terminal := base
	terminal.Status = TaskSucceeded
	check("terminal state", terminal, CodeStateConflict)

	notFound := base
	notFound.Found = false
	check("not found", notFound, CodeNotFound)

	crossIncident := base
	crossIncident.IncidentID = "inc-2"
	check("cross incident", crossIncident, CodeStateConflict)
}

// INV-09：Agent 结果只影响 Task，不影响 Incident 状态。
func TestINV09_ResultIsolation(t *testing.T) {
	typ := reflect.TypeOf(TaskEffect{})
	for i := 0; i < typ.NumField(); i++ {
		if strings.Contains(strings.ToLower(typ.Field(i).Name), "incident") {
			t.Fatalf("TaskEffect must not carry incident field: %s", typ.Field(i).Name)
		}
	}

	if e := ApplyTaskResult(OutcomeSucceeded, 0, 3, 0, 0); e.Status != TaskSucceeded {
		t.Fatalf("success should map Task SUCCEEDED, got %+v", e)
	}
	if e := ApplyTaskResult(OutcomeRetryableFailure, 0, 3, 1000, 60); e.Status != TaskRetryWait || e.NextRetryAt != 1060 {
		t.Fatalf("retryable should RETRY_WAIT, got %+v", e)
	}
	if e := ApplyTaskResult(OutcomeRetryableFailure, 2, 3, 0, 60); e.Status != TaskFailed {
		t.Fatalf("max attempts should FAILED, got %+v", e)
	}
}

// INV-12：终态不自动重开；RETRY_WAIT 到期可推进；人工重试生成新代次。
func TestINV12_TerminalAndRetry(t *testing.T) {
	if CanAutoPromote(TaskSucceeded, 0, 9999) {
		t.Fatal("SUCCEEDED must not auto-promote")
	}
	if CanAutoPromote(TaskFailed, 1000, 9999) {
		t.Fatal("FAILED must not auto-promote")
	}
	if !CanAutoPromote(TaskRetryWait, 1000, 1000) {
		t.Fatal("RETRY_WAIT due should promote")
	}
	if CanAutoPromote(TaskRetryWait, 2000, 1000) {
		t.Fatal("RETRY_WAIT not due should not promote")
	}

	p := PlanManualRetry(3, TaskFailed)
	if !p.OK || p.NewGeneration != 4 {
		t.Fatalf("manual retry plan from FAILED: %+v", p)
	}
	bad := PlanManualRetry(3, TaskRunning)
	if bad.OK || !IsCode(bad.Err, CodeStateConflict) {
		t.Fatalf("manual retry from non-terminal should reject: %+v", bad)
	}
}