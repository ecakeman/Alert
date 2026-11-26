package domain

import "testing"

// INV-06：任务只能读自己绑定的输入范围，越界/跨故障/反向范围一律拒绝。
func TestINV06_InputBound(t *testing.T) {
	bound := InputBound{IncidentID: "inc-1", From: 1, To: 5, MonitorWatermark: 100, AppWatermark: 50}

	if ok, r := CanRead(bound, "inc-1", 3, 40); !ok {
		t.Fatalf("expected allow, reason=%s", r)
	}
	if ok, r := CanRead(bound, "inc-1", 6, 40); ok || r != ReasonOutOfRange {
		t.Fatalf("version out of range: ok=%v reason=%s", ok, r)
	}
	if ok, r := CanRead(bound, "inc-2", 3, 40); ok || r != ReasonCrossIncident {
		t.Fatalf("cross incident: ok=%v reason=%s", ok, r)
	}
	if ok, r := CanRead(bound, "inc-1", 3, 200); ok || r != ReasonOutOfRange {
		t.Fatalf("watermark out of range: ok=%v reason=%s", ok, r)
	}

	bad := InputBound{IncidentID: "inc-1", From: 5, To: 1}
	if ok, r := CanRead(bad, "inc-1", 3, 20); ok || r != ReasonInvalidRange {
		t.Fatalf("reversed range: ok=%v reason=%s", ok, r)
	}
	if err := ValidateBound(bad); !IsCode(err, CodeInvalidRange) {
		t.Fatalf("ValidateBound reversed should be INVALID_RANGE, got %v", err)
	}
	if err := ValidateBound(bound); err != nil {
		t.Fatalf("ValidateBound valid should pass, got %v", err)
	}
}