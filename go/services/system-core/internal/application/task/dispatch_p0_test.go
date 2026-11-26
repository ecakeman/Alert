package task

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/ecakeman/alert/services/system-core/internal/application/apperr"
	portepisode "github.com/ecakeman/alert/services/system-core/internal/application/port/episode"
	portoutbox "github.com/ecakeman/alert/services/system-core/internal/application/port/outbox"
	portshared "github.com/ecakeman/alert/services/system-core/internal/application/port/shared"
	porttask "github.com/ecakeman/alert/services/system-core/internal/application/port/task"
	"github.com/ecakeman/alert/services/system-core/internal/contracts"
)

type p18UOW struct{}

func (p18UOW) WithinTx(ctx context.Context, fn func(context.Context, portshared.Tx) error) error {
	return fn(ctx, nil)
}

type p18Clock struct{}

func (p18Clock) Now() time.Time { return time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC) }

type p18IDs struct{ n int }

func (f *p18IDs) NewID() string { f.n++; return fmt.Sprintf("id-%d", f.n) }

type p18Idem struct {
	reserved map[string]bool
	results  map[string]string
}

func newP18Idem() *p18Idem {
	return &p18Idem{reserved: map[string]bool{}, results: map[string]string{}}
}

func (f *p18Idem) Reserve(_ context.Context, _ portshared.Tx, idem portoutbox.ConsumerIdempotency) (bool, error) {
	if f.reserved[idem.IdempotencyKey] {
		return false, nil
	}
	f.reserved[idem.IdempotencyKey] = true
	return true, nil
}

func (f *p18Idem) MarkDone(_ context.Context, _ portshared.Tx, _, key, resultCode string, _ time.Time) error {
	f.results[key] = resultCode
	return nil
}

type p18Tasks struct {
	porttask.TaskRepo
	created []porttask.AgentTask
	active  []porttask.AgentTask
}

func (f *p18Tasks) Create(_ context.Context, _ portshared.Tx, t porttask.AgentTask) error {
	f.created = append(f.created, t)
	return nil
}

func (f *p18Tasks) ListActiveByEpisode(context.Context, portshared.Tx, string) ([]porttask.AgentTask, error) {
	return f.active, nil
}

type p18Outbox struct {
	portoutbox.OutboxRepo
	events []contracts.Event
}

func (f *p18Outbox) Append(_ context.Context, _ portshared.Tx, e contracts.Event, _ *string) error {
	f.events = append(f.events, e)
	return nil
}

type p18PanicEpisodes struct {
	portepisode.EpisodeRepo
	called *bool
}

func (p p18PanicEpisodes) Get(context.Context, portshared.Tx, string) (portepisode.Episode, error) {
	if p.called != nil {
		*p.called = true
	}
	panic("episode lookup must not happen in dispatch path")
}

func (p p18PanicEpisodes) GetActiveByService(context.Context, portshared.Tx, string) (portepisode.Episode, error) {
	if p.called != nil {
		*p.called = true
	}
	panic("episode lookup must not happen in dispatch path")
}

type p18Dispatch struct {
	snapshots []contracts.DispatchSnapshot
}

func (f p18Dispatch) ListPendingDispatch(context.Context, int) ([]contracts.DispatchSnapshot, error) {
	return f.snapshots, nil
}

type p18Harness struct {
	svc    *Service
	tasks  *p18Tasks
	idem   *p18Idem
	outbox *p18Outbox
	lookup bool
}

func newP18Harness() *p18Harness {
	h := &p18Harness{
		tasks:  &p18Tasks{},
		idem:   newP18Idem(),
		outbox: &p18Outbox{},
	}
	h.svc = New(Deps{
		UOW:         p18UOW{},
		Tasks:       h.tasks,
		Episodes:    p18PanicEpisodes{called: &h.lookup},
		Idem:        h.idem,
		Outbox:      h.outbox,
		Clock:       p18Clock{},
		IDs:         &p18IDs{},
		Deadline:    time.Minute,
		MaxAttempts: 3,
	})
	return h
}

func activeSnapshot() contracts.DispatchSnapshot {
	return contracts.DispatchSnapshot{
		EpisodeID: "ep-1", ServiceID: "svc-1", EpisodeStatus: "ACTIVE",
		InputVersionFrom: 1, InputVersionTo: 2, Reason: "INPUT_APPENDED",
	}
}

func TestP0DispatchableDeliveredThreeTimesCreatesOneTask(t *testing.T) {
	h := newP18Harness()
	event := contracts.Event{
		EventID:   "event-1",
		EventType: contracts.EventEpisodeDispatchable,
		Payload:   mustJSON(t, activeSnapshot()),
	}
	for i := 0; i < 3; i++ {
		if err := h.svc.HandleDispatchableEvent(context.Background(), event); err != nil {
			t.Fatalf("第 %d 次投递失败: %v", i+1, err)
		}
	}
	if len(h.tasks.created) != 1 {
		t.Fatalf("任务数 = %d，期望 1", len(h.tasks.created))
	}
	if len(h.outbox.events) != 1 || h.outbox.events[0].EventType != contracts.EventTaskDispatchRequested {
		t.Fatalf("派发事件 = %+v，期望 1 条 task.dispatch.requested", h.outbox.events)
	}
	if h.lookup {
		t.Fatal("主路径不应回查 episode")
	}
}

func TestP0DispatchableMissingFieldsIsValidation(t *testing.T) {
	h := newP18Harness()
	snapshot := activeSnapshot()
	snapshot.ServiceID = ""

	_, _, err := h.svc.Create(context.Background(), CreateInput{
		Snapshot: snapshot, TaskType: TaskTypeDiagnosis, Source: SourceEpisodeActive,
	})
	if apperr.Code(err) != apperr.CodeValidation {
		t.Fatalf("错误码 = %s，期望 VALIDATION", apperr.Code(err))
	}
	if len(h.tasks.created) != 0 || len(h.outbox.events) != 0 {
		t.Fatal("快照非法时不应建任务或写事件")
	}
}

func TestP0ArchiveSkippedWhenArtifactsNotReady(t *testing.T) {
	h := newP18Harness()
	snapshot := activeSnapshot()
	snapshot.EpisodeStatus = "RESOLVED"
	snapshot.Reason = "EPISODE_RESOLVED"

	view, reused, err := h.svc.Create(context.Background(), CreateInput{
		Snapshot: snapshot, TaskType: TaskTypeArchive, Source: SourceEpisodeResolved,
	})
	if err != nil {
		t.Fatalf("Create 失败: %v", err)
	}
	if view.TaskID != "" || reused {
		t.Fatalf("产物未就绪不应建任务: view=%+v reused=%v", view, reused)
	}
	if len(h.tasks.created) != 0 {
		t.Fatal("产物未就绪时任务数应为 0")
	}
	key := fmt.Sprintf("episode:%s:%d:%s", snapshot.EpisodeID, snapshot.InputVersionTo, snapshot.Reason)
	if h.idem.results[key] != "SKIPPED_NOT_READY" {
		t.Fatalf("幂等完成码 = %q", h.idem.results[key])
	}
}

func TestP0ReconcileRecreatesMissingTaskOnce(t *testing.T) {
	h := newP18Harness()
	h.svc.deps.Dispatch = p18Dispatch{snapshots: []contracts.DispatchSnapshot{activeSnapshot()}}

	created, err := h.svc.EnsureDiagnosisTasks(context.Background(), 10)
	if err != nil {
		t.Fatalf("对账失败: %v", err)
	}
	if created != 1 || len(h.tasks.created) != 1 {
		t.Fatalf("首次对账 created=%d tasks=%d，期望 1", created, len(h.tasks.created))
	}

	created, err = h.svc.EnsureDiagnosisTasks(context.Background(), 10)
	if err != nil {
		t.Fatalf("二次对账失败: %v", err)
	}
	if created != 0 || len(h.tasks.created) != 1 {
		t.Fatalf("二次对账不应重复建任务: created=%d tasks=%d", created, len(h.tasks.created))
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	return body
}
