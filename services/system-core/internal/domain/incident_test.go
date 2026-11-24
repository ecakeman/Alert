package domain

import "testing"

// INV-01：同 Service 已有 ACTIVE 则复用，不再新建。
func TestINV01_SingleActiveIncident(t *testing.T) {
	if d := DecideIncidentOnAbnormal(true, "inc-1"); d.Kind != IncidentReuse || d.IncidentID != "inc-1" {
		t.Fatalf("expected reuse inc-1, got %+v", d)
	}
	if d := DecideIncidentOnAbnormal(false, ""); d.Kind != IncidentCreate {
		t.Fatalf("expected create, got %+v", d)
	}
	// 已归档不算活动：调用方传 activeExists=false。
	if d := DecideIncidentOnAbnormal(false, "archived"); d.Kind != IncidentCreate {
		t.Fatalf("archived must not be treated as active, got %+v", d)
	}
}

// INV-03：解除需全部条件满足；缺任一即拒绝并列出缺失项。
func TestINV03_CanResolve(t *testing.T) {
	if ok, _ := CanResolve(ResolveInput{MonitorNormal: true, AppNormal: true, FinalInputProcessed: true}); !ok {
		t.Fatal("all satisfied should resolve")
	}

	cases := []struct {
		in   ResolveInput
		want MissingCondition
	}{
		{ResolveInput{MonitorNormal: false, AppNormal: true, FinalInputProcessed: true}, MissingMonitorNormal},
		{ResolveInput{MonitorNormal: true, AppNormal: false, FinalInputProcessed: true}, MissingAppNormal},
		{ResolveInput{MonitorNormal: true, AppNormal: true, FinalInputProcessed: false}, MissingFinalInputProcessed},
		{ResolveInput{MonitorNormal: true, AppNormal: true, FinalInputProcessed: true, ActiveTaskExists: true}, MissingNoActiveTask},
	}
	for _, c := range cases {
		resolved, missing := CanResolve(c.in)
		if resolved {
			t.Fatalf("expected not resolved for %+v", c.in)
		}
		found := false
		for _, m := range missing {
			if m == c.want {
				found = true
			}
		}
		if !found {
			t.Fatalf("expected missing %s for %+v, got %v", c.want, c.in, missing)
		}
	}

	// UNKNOWN 参与解除判定时必须拒绝。
	resolved, _ := CanResolve(ResolveInput{
		MonitorNormal:       IsNormal(ParseSourceStatus("UNKNOWN")),
		AppNormal:           true,
		FinalInputProcessed: true,
	})
	if resolved {
		t.Fatal("UNKNOWN monitor must not resolve")
	}
}