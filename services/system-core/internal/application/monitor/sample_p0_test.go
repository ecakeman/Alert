package monitor

import (
	"context"
	"errors"
	"testing"
	"time"

	portepisode "github.com/ecakeman/alert/services/system-core/internal/application/port/episode"
	portmonitor "github.com/ecakeman/alert/services/system-core/internal/application/port/monitor"
	portoutbox "github.com/ecakeman/alert/services/system-core/internal/application/port/outbox"
	portService "github.com/ecakeman/alert/services/system-core/internal/application/port/service"
	portshared "github.com/ecakeman/alert/services/system-core/internal/application/port/shared"
	"github.com/ecakeman/alert/services/system-core/internal/contracts"
)

type fakeP0UOW struct{}

func (fakeP0UOW) WithinTx(ctx context.Context, fn func(context.Context, portshared.Tx) error) error {
	return fn(ctx, nil)
}

type fakeClock struct{ now time.Time }

func (c fakeClock) Now() time.Time { return c.now }

type fakeIDs struct{ n int }

func (f *fakeIDs) NewID() string {
	f.n++
	return "id-" + time.Duration(f.n).String()
}

type fakeTargets struct {
	portService.TargetRepo
	target       portService.MonitorTarget
	targets      []portService.MonitorTarget
	updateErr    error
	probeUpdates int
	lastCheckAt  time.Time
}

func (f *fakeTargets) Get(context.Context, portshared.Tx, string) (portService.MonitorTarget, error) {
	return f.target, nil
}

func (f *fakeTargets) ListByService(context.Context, portshared.Tx, string, bool) ([]portService.MonitorTarget, error) {
	out := make([]portService.MonitorTarget, len(f.targets))
	copy(out, f.targets)
	return out, nil
}

func (f *fakeTargets) UpdateProbeResult(_ context.Context, _ portshared.Tx, _ string, status string, failures, successes int, at time.Time) error {
	f.probeUpdates++
	if f.updateErr != nil {
		return f.updateErr
	}
	f.target.Status = status
	f.target.ConsecutiveFailures = failures
	f.target.ConsecutiveSuccesses = successes
	f.target.LastCheckAt = &at
	for i := range f.targets {
		if f.targets[i].TargetID == f.target.TargetID {
			f.targets[i] = f.target
		}
	}
	return nil
}

func (f *fakeTargets) UpdateLastCheckAt(_ context.Context, _ portshared.Tx, _ string, at time.Time) error {
	f.lastCheckAt = at
	return nil
}

type fakePoints struct {
	portmonitor.MonitorPointRepo
	appended []portmonitor.MonitorPoint
}

func (f *fakePoints) Append(_ context.Context, _ portshared.Tx, p portmonitor.MonitorPoint) (int64, error) {
	f.appended = append(f.appended, p)
	return int64(len(f.appended)), nil
}

type fakeSources struct {
	portService.SourceStateRepo
	states  []portService.SourceState
	upserts []portService.SourceState
}

func (f *fakeSources) ListByService(context.Context, portshared.Tx, string) ([]portService.SourceState, error) {
	out := make([]portService.SourceState, len(f.states))
	copy(out, f.states)
	return out, nil
}

func (f *fakeSources) Upsert(_ context.Context, _ portshared.Tx, s portService.SourceState) error {
	f.upserts = append(f.upserts, s)
	return nil
}

type fakeServices struct {
	portService.ServiceRepo
	mode string
}

func (f fakeServices) Get(context.Context, portshared.Tx, string) (portService.Service, error) {
	return portService.Service{MonitorAggregationMode: f.mode}, nil
}

type fakeEpisodes struct {
	portepisode.EpisodeRepo
	active portepisode.Episode
	err    error
}

func (f fakeEpisodes) GetActiveByService(context.Context, portshared.Tx, string) (portepisode.Episode, error) {
	return f.active, f.err
}

type fakeOutbox struct {
	portoutbox.OutboxRepo
	events []contracts.Event
}

func (f *fakeOutbox) Append(_ context.Context, _ portshared.Tx, e contracts.Event, _ *string) error {
	f.events = append(f.events, e)
	return nil
}

func (f *fakeOutbox) count(eventType string) int {
	n := 0
	for _, e := range f.events {
		if e.EventType == eventType {
			n++
		}
	}
	return n
}

type fakeDriver struct {
	openCalls    int
	resolveCalls int
	err          error
}

func (f *fakeDriver) OpenOrReuse(context.Context, string) (bool, error) {
	f.openCalls++
	return false, f.err
}

func (f *fakeDriver) TryResolve(context.Context, string) (bool, []string, error) {
	f.resolveCalls++
	return false, nil, f.err
}

type harness struct {
	svc     *Service
	targets *fakeTargets
	points  *fakePoints
	sources *fakeSources
	outbox  *fakeOutbox
	driver  *fakeDriver
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	target := portService.MonitorTarget{
		TargetID: "t1", ServiceID: "s1", TargetKey: "http-primary", TargetType: "HTTP",
		FailureThreshold: 1, RecoveryThreshold: 1, IsRequired: true, Enabled: true,
		Status: "NORMAL", ConfigGeneration: 1,
	}
	f := &harness{
		targets: &fakeTargets{target: target, targets: []portService.MonitorTarget{target}},
		points:  &fakePoints{},
		sources: &fakeSources{states: []portService.SourceState{{ServiceID: "s1", SourceType: "MONITOR", Status: "NORMAL"}}},
		outbox:  &fakeOutbox{},
		driver:  &fakeDriver{},
	}
	f.svc = New(Deps{
		UOW:           fakeP0UOW{},
		Targets:       f.targets,
		MonitorPoints: f.points,
		Sources:       f.sources,
		Episodes:      fakeEpisodes{active: portepisode.Episode{EpisodeID: "e1"}},
		Services:      fakeServices{mode: "ALL_REQUIRED"},
		Outbox:        f.outbox,
		Driver:        f.driver,
		Clock:         fakeClock{now: time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)},
		IDs:           &fakeIDs{},
	})
	return f
}

func TestP0SampleAbnormalPersistsPointsAndEvents(t *testing.T) {
	f := newHarness(t)
	status, err := f.svc.ApplySample(context.Background(), "s1", "t1", 1, Sample{CheckStatus: "FAILURE"})
	if err != nil {
		t.Fatalf("ApplySample 失败: %v", err)
	}
	if status != "ABNORMAL" {
		t.Fatalf("source status = %q", status)
	}
	if len(f.points.appended) != 1 {
		t.Fatalf("points = %d，期望 1", len(f.points.appended))
	}
	if f.targets.probeUpdates != 1 || f.targets.target.Status != "ABNORMAL" {
		t.Fatalf("target 更新异常: updates=%d status=%s", f.targets.probeUpdates, f.targets.target.Status)
	}
	if len(f.sources.upserts) != 1 || f.sources.upserts[0].Status != "ABNORMAL" {
		t.Fatalf("source upserts = %+v", f.sources.upserts)
	}
	if got := f.outbox.count(contracts.EventMonitorTargetStateChanged); got != 1 {
		t.Fatalf("target 事件数 = %d，期望 1", got)
	}
	if got := f.outbox.count(contracts.EventMonitorServiceStateChanged); got != 1 {
		t.Fatalf("service 事件数 = %d，期望 1", got)
	}
	if f.driver.openCalls != 1 {
		t.Fatalf("OpenOrReuse 调用 %d 次，期望 1", f.driver.openCalls)
	}
}

func TestP0SampleFiveUnchangedEmitsNoMonitorEvents(t *testing.T) {
	f := newHarness(t)
	f.targets.target.Status = "ABNORMAL"
	f.targets.target.ConsecutiveFailures = 5
	f.targets.target.FailureThreshold = 5
	f.targets.targets = []portService.MonitorTarget{f.targets.target}
	f.sources.states = []portService.SourceState{{ServiceID: "s1", SourceType: "MONITOR", Status: "ABNORMAL"}}
	f.driver.openCalls = 0

	for i := 0; i < 5; i++ {
		if _, err := f.svc.ApplySample(context.Background(), "s1", "t1", 1, Sample{CheckStatus: "FAILURE"}); err != nil {
			t.Fatalf("第 %d 次采样失败: %v", i+1, err)
		}
	}
	if len(f.points.appended) != 5 {
		t.Fatalf("points = %d，期望 5", len(f.points.appended))
	}
	if len(f.outbox.events) != 0 {
		t.Fatalf("无变化采样不应写事件，实际 %d 条", len(f.outbox.events))
	}
	if f.driver.openCalls != 0 {
		t.Fatalf("无变化不应驱动 episode，实际 %d 次", f.driver.openCalls)
	}
}

func TestP0OutOfTxFailureStillPersistsLocalWrites(t *testing.T) {
	f := newHarness(t)
	f.targets.updateErr = errors.New("registry unavailable")

	status, err := f.svc.ApplySample(context.Background(), "s1", "t1", 1, Sample{CheckStatus: "FAILURE"})
	if err != nil {
		t.Fatalf("事务外失败不应使采样返回错误: %v", err)
	}
	if status != "ABNORMAL" {
		t.Fatalf("source status = %q", status)
	}
	if len(f.points.appended) != 1 {
		t.Fatalf("points 未落库")
	}
	if f.outbox.count(contracts.EventMonitorTargetStateChanged) != 1 || f.outbox.count(contracts.EventMonitorServiceStateChanged) != 1 {
		t.Fatalf("事件未落 outbox: %d/%d",
			f.outbox.count(contracts.EventMonitorTargetStateChanged),
			f.outbox.count(contracts.EventMonitorServiceStateChanged))
	}
	if got := f.svc.OutOfTxFailures(); got != 1 {
		t.Fatalf("失败计数 = %d，期望 1", got)
	}

	// 下一轮恢复后应能修正 target 状态。
	f.targets.updateErr = nil
	if _, err := f.svc.ApplySample(context.Background(), "s1", "t1", 1, Sample{CheckStatus: "SUCCESS"}); err != nil {
		t.Fatalf("恢复采样失败: %v", err)
	}
	if f.targets.target.Status != "NORMAL" {
		t.Fatalf("恢复后 target status = %s，期望 NORMAL", f.targets.target.Status)
	}
}

func TestP0GenerationMismatchSkipsEverything(t *testing.T) {
	f := newHarness(t)
	status, err := f.svc.ApplySample(context.Background(), "s1", "t1", 2, Sample{CheckStatus: "FAILURE"})
	if err != nil {
		t.Fatalf("ApplySample 失败: %v", err)
	}
	if status != "NORMAL" {
		t.Fatalf("status = %q，期望沿用当前值 NORMAL", status)
	}
	if len(f.points.appended) != 0 || len(f.outbox.events) != 0 || f.targets.probeUpdates != 0 {
		t.Fatalf("generation 不匹配时不应写任何数据")
	}
}
