package episode

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ecakeman/alert/services/system-core/internal/application/apperr"
	portepisode "github.com/ecakeman/alert/services/system-core/internal/application/port/episode"
	portoutbox "github.com/ecakeman/alert/services/system-core/internal/application/port/outbox"
	portService "github.com/ecakeman/alert/services/system-core/internal/application/port/service"
	portshared "github.com/ecakeman/alert/services/system-core/internal/application/port/shared"
	porttask "github.com/ecakeman/alert/services/system-core/internal/application/port/task"
	"github.com/ecakeman/alert/services/system-core/internal/contracts"
)

type fakeP17UOW struct{}

func (fakeP17UOW) WithinTx(ctx context.Context, fn func(context.Context, portshared.Tx) error) error {
	return fn(ctx, nil)
}

type fakeP17Clock struct{}

func (fakeP17Clock) Now() time.Time { return time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC) }

type fakeP17IDs struct{ n int }

func (f *fakeP17IDs) NewID() string { f.n++; return "id-" + string(rune('0'+f.n)) }

type fakeP17Episodes struct {
	portepisode.EpisodeRepo
	mu           sync.Mutex
	ep           portepisode.Episode
	getErr       error
	resolveErr   error
	casResult    *bool
	resolvedOnce bool
	resolveCalls int
	updateStatus int
}

func (f *fakeP17Episodes) Get(context.Context, portshared.Tx, string) (portepisode.Episode, error) {
	return f.ep, f.getErr
}

func (f *fakeP17Episodes) ResolveIfUnchanged(_ context.Context, _ portshared.Tx, _ string, expectedUpdatedAt, _ time.Time) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resolveCalls++
	if f.resolveErr != nil {
		return false, f.resolveErr
	}
	if f.casResult != nil {
		return *f.casResult, nil
	}
	if f.resolvedOnce || !expectedUpdatedAt.Equal(f.ep.UpdatedAt) {
		return false, nil
	}
	f.resolvedOnce = true
	return true, nil
}

func (f *fakeP17Episodes) UpdateStatus(context.Context, portshared.Tx, string, string, time.Time, *time.Time, *time.Time) error {
	f.updateStatus++
	return nil
}

type fakeP17Sources struct {
	portService.SourceStateRepo
	states []portService.SourceState
	err    error
}

func (f fakeP17Sources) ListByService(context.Context, portshared.Tx, string) ([]portService.SourceState, error) {
	return f.states, f.err
}

type fakeP17Tasks struct {
	porttask.TaskRepo
	active []porttask.AgentTask
	err    error
}

func (f fakeP17Tasks) ListActiveByEpisode(context.Context, portshared.Tx, string) ([]porttask.AgentTask, error) {
	return f.active, f.err
}

type fakeP17Outbox struct {
	portoutbox.OutboxRepo
	events []contracts.Event
}

func (f *fakeP17Outbox) Append(_ context.Context, _ portshared.Tx, e contracts.Event, _ *string) error {
	f.events = append(f.events, e)
	return nil
}

func newResolveHarness(t *testing.T) (*Service, *fakeP17Episodes, *fakeP17Outbox) {
	t.Helper()
	episodes := &fakeP17Episodes{ep: portepisode.Episode{
		EpisodeID:             "ep-1",
		ServiceID:             "svc-1",
		Status:                "ACTIVE",
		FinalInputVersion:     2,
		ProcessedInputVersion: 2,
		UpdatedAt:             time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC),
	}}
	outbox := &fakeP17Outbox{}
	svc := New(Deps{
		UOW:      fakeP17UOW{},
		Episodes: episodes,
		Sources: fakeP17Sources{states: []portService.SourceState{
			{ServiceID: "svc-1", SourceType: "MONITOR", Status: "NORMAL"},
			{ServiceID: "svc-1", SourceType: "APP", Status: "NORMAL"},
		}},
		Tasks:  fakeP17Tasks{},
		Outbox: outbox,
		Clock:  fakeP17Clock{},
		IDs:    &fakeP17IDs{},
	})
	return svc, episodes, outbox
}

func TestP0ResolveConcurrentOnlyOneSucceeds(t *testing.T) {
	svc, episodes, outbox := newResolveHarness(t)

	type result struct {
		resolved bool
		err      error
	}
	results := make(chan result, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resolved, _, err := svc.TryResolve(context.Background(), "ep-1")
			results <- result{resolved: resolved, err: err}
		}()
	}
	wg.Wait()
	close(results)

	success := 0
	for res := range results {
		if res.err != nil {
			t.Fatalf("并发解除返回错误: %v", res.err)
		}
		if res.resolved {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("成功解除数 = %d，期望 1", success)
	}
	if episodes.resolveCalls != 2 {
		t.Fatalf("CAS 调用 = %d，期望 2", episodes.resolveCalls)
	}
	if len(outbox.events) != 1 || outbox.events[0].EventType != contracts.EventEpisodeStatusChanged {
		t.Fatalf("事件 = %+v，期望 1 条 status.changed", outbox.events)
	}
}

func TestP0ResolveMissingOrderUnchanged(t *testing.T) {
	svc, episodes, outbox := newResolveHarness(t)
	svc.deps.Sources = fakeP17Sources{states: []portService.SourceState{
		{ServiceID: "svc-1", SourceType: "MONITOR", Status: "ABNORMAL"},
		{ServiceID: "svc-1", SourceType: "APP", Status: "ABNORMAL"},
	}}
	episodes.ep.FinalInputVersion = 3
	episodes.ep.ProcessedInputVersion = 2
	svc.deps.Tasks = fakeP17Tasks{active: []porttask.AgentTask{{TaskID: "task-1"}}}

	resolved, missing, err := svc.TryResolve(context.Background(), "ep-1")
	if err != nil {
		t.Fatalf("TryResolve 失败: %v", err)
	}
	if resolved {
		t.Fatal("条件不足时不应解除")
	}
	want := []string{"MONITOR_NOT_NORMAL", "APP_NOT_NORMAL", "FINAL_INPUT_NOT_PROCESSED", "ACTIVE_TASK_EXISTS"}
	if len(missing) != len(want) {
		t.Fatalf("missing = %v，期望 %v", missing, want)
	}
	for i := range want {
		if missing[i] != want[i] {
			t.Fatalf("missing[%d] = %q，期望 %q", i, missing[i], want[i])
		}
	}
	if episodes.resolveCalls != 0 || len(outbox.events) != 0 {
		t.Fatalf("条件不足时不应调用 CAS 或写事件")
	}
}

func TestP0ResolveCASConflictIsNotError(t *testing.T) {
	svc, episodes, outbox := newResolveHarness(t)
	conflict := false
	episodes.casResult = &conflict

	resolved, missing, err := svc.TryResolve(context.Background(), "ep-1")
	if err != nil {
		t.Fatalf("CAS 冲突不应返回错误: %v", err)
	}
	if resolved {
		t.Fatal("CAS 冲突时不应声称解除成功")
	}
	if len(missing) != 0 {
		t.Fatalf("条件满足时 missing 应为空: %v", missing)
	}
	if len(outbox.events) != 0 {
		t.Fatalf("CAS 冲突时不应写事件")
	}
}

func TestP0ResolvePrecheckFailureReturnsDependencyUnavailable(t *testing.T) {
	svc, episodes, _ := newResolveHarness(t)
	svc.deps.Sources = fakeP17Sources{err: errors.New("registry down")}

	resolved, missing, err := svc.TryResolve(context.Background(), "ep-1")
	if resolved || len(missing) != 0 {
		t.Fatalf("预检失败时不应推进状态: resolved=%v missing=%v", resolved, missing)
	}
	if apperr.Code(err) != apperr.CodeDependencyUnavailable {
		t.Fatalf("错误码 = %s，期望 DEPENDENCY_UNAVAILABLE", apperr.Code(err))
	}
	if episodes.resolveCalls != 0 || episodes.updateStatus != 0 {
		t.Fatalf("预检失败后不应写状态")
	}
}
