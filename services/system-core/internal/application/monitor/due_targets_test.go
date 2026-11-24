package monitor

import (
	"context"
	"testing"
	"time"

	portService "github.com/ecakeman/alert/services/system-core/internal/application/port/service"
	portshared "github.com/ecakeman/alert/services/system-core/internal/application/port/shared"
)

type fakeTargetRepo struct{ all []portService.MonitorTarget }

func (f fakeTargetRepo) ListEnabled(ctx context.Context, tx portshared.Tx) ([]portService.MonitorTarget, error) {
	var out []portService.MonitorTarget
	for _, t := range f.all {
		if t.Enabled { // 模拟 ListEnabled SQL：已排除禁用/删除
			out = append(out, t)
		}
	}
	return out, nil
}

func (fakeTargetRepo) Create(context.Context, portshared.Tx, portService.MonitorTarget) error {
	return nil
}
func (fakeTargetRepo) Get(context.Context, portshared.Tx, string) (portService.MonitorTarget, error) {
	return portService.MonitorTarget{}, nil
}
func (fakeTargetRepo) ListByService(context.Context, portshared.Tx, string, bool) ([]portService.MonitorTarget, error) {
	return nil, nil
}
func (fakeTargetRepo) Patch(context.Context, portshared.Tx, string, portService.TargetPatch) error {
	return nil
}
func (fakeTargetRepo) UpdateProbeResult(context.Context, portshared.Tx, string, string, int, int, time.Time) error {
	return nil
}
func (fakeTargetRepo) SoftDelete(context.Context, portshared.Tx, string, time.Time) error { return nil }

type fakeUOW struct{}

func (fakeUOW) WithinTx(ctx context.Context, fn func(context.Context, portshared.Tx) error) error {
	return fn(ctx, nil)
}

func ptrTime(t time.Time) *time.Time { return &t }

// TestDueTargets 覆盖三类：到期（含 LastCheckAt 为空）、未到期、禁用不返回。
func TestDueTargets(t *testing.T) {
	now := time.Now().UTC()

	due := portService.MonitorTarget{
		TargetID: "t1", ServiceID: "s1", TargetKey: "ok", TargetType: "HTTP",
		Address: "http://x", IntervalMS: 1000, TimeoutMS: 500,
		FailureThreshold: 3, RecoveryThreshold: 2, IsRequired: true,
		Enabled: true, Status: "NORMAL", ConfigGeneration: 1,
		LastCheckAt: ptrTime(now.Add(-2 * time.Second)), // 1s 间隔，已到期
	}
	notDue := due
	notDue.TargetID = "t2"
	notDue.TargetKey = "fresh"
	notDue.LastCheckAt = ptrTime(now) // 刚探测过，未到期
	disabled := due
	disabled.TargetID = "t3"
	disabled.TargetKey = "off"
	disabled.Enabled = false
	disabled.LastCheckAt = ptrTime(now.Add(-10 * time.Second))
	neverChecked := due
	neverChecked.TargetID = "t4"
	neverChecked.TargetKey = "new"
	neverChecked.LastCheckAt = nil // 从未探测，视为到期

	uc := New(Deps{UOW: fakeUOW{}, Targets: fakeTargetRepo{all: []portService.MonitorTarget{due, notDue, disabled, neverChecked}}})

	snaps, err := uc.DueTargets(context.Background(), now)
	if err != nil {
		t.Fatalf("DueTargets: %v", err)
	}

	if len(snaps) != 2 {
		t.Fatalf("want 2 due snapshots, got %d: %+v", len(snaps), snaps)
	}
	ids := map[string]bool{}
	for _, s := range snaps {
		ids[s.TargetID] = true
	}
	if !ids["t1"] || !ids["t4"] {
		t.Fatalf("due/new-never-checked should be included: %v", ids)
	}
	if ids["t2"] {
		t.Fatal("not-due target must be excluded")
	}
	if ids["t3"] {
		t.Fatal("disabled target must not be returned")
	}

	for _, s := range snaps {
		if s.ServiceID != "s1" || s.Address != "http://x" || s.ConfigGeneration != 1 {
			t.Fatalf("snapshot fields incomplete: %+v", s)
		}
	}
}
