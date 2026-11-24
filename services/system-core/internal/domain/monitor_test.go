package domain

import "testing"

func TestAggregateMonitor(t *testing.T) {
	n := TargetHealth{Required: true, Enabled: true, Status: "NORMAL"}
	a := TargetHealth{Required: true, Enabled: true, Status: "ABNORMAL"}
	u := TargetHealth{Required: true, Enabled: true, Status: "UNKNOWN"}
	optional := TargetHealth{Required: false, Enabled: true, Status: "ABNORMAL"}
	disabled := TargetHealth{Required: true, Enabled: false, Status: "ABNORMAL"}
	deleted := TargetHealth{Required: true, Enabled: true, Deleted: true, Status: "ABNORMAL"}

	cases := []struct {
		name    string
		mode    string
		targets []TargetHealth
		want    string
	}{
		{"all: all normal", ModeAllRequired, []TargetHealth{n, n}, "NORMAL"},
		{"all: any abnormal wins", ModeAllRequired, []TargetHealth{n, a}, "ABNORMAL"},
		{"all: unknown blocks normal", ModeAllRequired, []TargetHealth{n, u}, "UNKNOWN"},
		{"any: normal wins", ModeAnyRequired, []TargetHealth{n, a}, "NORMAL"},
		{"any: all abnormal", ModeAnyRequired, []TargetHealth{a, a}, "ABNORMAL"},
		{"any: abnormal+unknown", ModeAnyRequired, []TargetHealth{a, u}, "UNKNOWN"},
		{"majority: normal wins", ModeMajority, []TargetHealth{n, n, a}, "NORMAL"},
		{"majority: abnormal wins", ModeMajority, []TargetHealth{a, a, n}, "ABNORMAL"},
		{"majority: split with unknown", ModeMajority, []TargetHealth{n, a, u}, "UNKNOWN"},
		{"majority: unknown not counted as normal", ModeMajority, []TargetHealth{n, u, u}, "UNKNOWN"},
	}
	for _, c := range cases {
		got, err := AggregateMonitor(c.mode, c.targets)
		if err != nil {
			t.Fatalf("%s: unexpected err %v", c.name, err)
		}
		if got != c.want {
			t.Fatalf("%s: want %s got %s", c.name, c.want, got)
		}
	}

	// 空集合：nil、全部被过滤（非必需/禁用/已删除）→ UNKNOWN。
	if got, err := AggregateMonitor(ModeAllRequired, nil); err != nil || got != "UNKNOWN" {
		t.Fatalf("nil targets: want UNKNOWN got %s err %v", got, err)
	}
	if got, err := AggregateMonitor(ModeAnyRequired, []TargetHealth{optional, disabled, deleted}); err != nil || got != "UNKNOWN" {
		t.Fatalf("filtered-out only: want UNKNOWN got %s err %v", got, err)
	}

	// 未知模式 → CodeOutOfRange。
	_, err := AggregateMonitor("MAJORITY_X", []TargetHealth{n})
	if !IsCode(err, CodeOutOfRange) {
		t.Fatalf("unknown mode should be OUT_OF_RANGE, got %v", err)
	}
}
