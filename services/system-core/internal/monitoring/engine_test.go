package monitoring

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ecakeman/alertflow/services/system-core/internal/monitoring/probe/kubernetes"
	"github.com/ecakeman/alertflow/services/system-core/internal/system"
	"github.com/google/uuid"
)

type fakeStore struct {
	mu       sync.Mutex
	targets  []system.DueTarget
	applied  []system.MonitorSample
	inflight atomic.Int32
	max      atomic.Int32
}

func (f *fakeStore) ListDueTargets(context.Context) ([]system.DueTarget, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := append([]system.DueTarget(nil), f.targets...)
	return out, nil
}

func (f *fakeStore) ApplyMonitorSample(_ context.Context, sample system.MonitorSample) (system.ApplyResult, error) {
	f.mu.Lock()
	f.applied = append(f.applied, sample)
	f.mu.Unlock()
	return system.ApplyResult{}, nil
}

func (f *fakeStore) set(targets ...system.DueTarget) {
	f.mu.Lock()
	f.targets = targets
	f.mu.Unlock()
}

func target(id uuid.UUID, address string) system.DueTarget {
	return system.DueTarget{TargetID: id, TargetType: "HTTP", Address: address, Interval: time.Hour, Timeout: time.Second}
}

func TestQueueFullTargetIsRetried(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	store := &fakeStore{}
	store.set(target(a, "a"), target(b, "b"))
	release := make(chan struct{})
	var ran sync.Map
	old := execute
	execute = func(ctx context.Context, item system.DueTarget, _ *kubernetes.K8sObserver) (system.MonitorSample, error) {
		ran.Store(item.TargetID, item.Address)
		if item.TargetID == a {
			<-release
		}
		return system.MonitorSample{TargetID: item.TargetID, Success: true}, nil
	}
	t.Cleanup(func() { execute = old })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		run(ctx, store, nil, nil, Config{Workers: 1, QueueSize: 1, Reconcile: time.Hour, Tick: 5 * time.Millisecond}, nil)
		close(done)
	}()
	waitUntil(t, func() bool {
		_, ok := ran.Load(a)
		return ok
	})
	time.Sleep(30 * time.Millisecond)
	if _, ok := ran.Load(b); ok {
		t.Fatal("second target entered a full queue")
	}
	close(release)
	waitUntil(t, func() bool {
		_, ok := ran.Load(b)
		return ok
	})
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("engine did not exit after cancel")
	}
}

func TestSameTargetDoesNotOverlap(t *testing.T) {
	id := uuid.New()
	store := &fakeStore{}
	store.set(target(id, "once"))
	started := make(chan struct{})
	release := make(chan struct{})
	old := execute
	execute = func(ctx context.Context, item system.DueTarget, _ *kubernetes.K8sObserver) (system.MonitorSample, error) {
		current := store.inflight.Add(1)
		for {
			max := store.max.Load()
			if current <= max || store.max.CompareAndSwap(max, current) {
				break
			}
		}
		if current == 1 {
			close(started)
			<-release
		}
		store.inflight.Add(-1)
		return system.MonitorSample{TargetID: item.TargetID, Success: true}, nil
	}
	t.Cleanup(func() { execute = old })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go run(ctx, store, nil, nil, Config{Workers: 2, QueueSize: 2, Reconcile: 5 * time.Millisecond, Tick: 5 * time.Millisecond}, nil)
	<-started
	time.Sleep(40 * time.Millisecond)
	if store.max.Load() != 1 {
		t.Fatalf("overlapping probes = %d", store.max.Load())
	}
	close(release)
	cancel()
}

func TestStaleConfigIsSkipped(t *testing.T) {
	running := uuid.New()
	queued := uuid.New()
	store := &fakeStore{}
	hold := target(running, "hold")
	next := target(queued, "old")
	hold.Interval = 10 * time.Millisecond
	next.Interval = 10 * time.Millisecond
	store.set(hold, next)
	started := make(chan struct{})
	release := make(chan struct{})
	old := execute
	execute = func(ctx context.Context, item system.DueTarget, _ *kubernetes.K8sObserver) (system.MonitorSample, error) {
		if item.TargetID == running {
			select {
			case <-started:
			default:
				close(started)
			}
			<-release
		}
		return system.MonitorSample{TargetID: item.TargetID, Success: true, StatusClass: item.Address}, nil
	}
	t.Cleanup(func() { execute = old })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go run(ctx, store, nil, nil, Config{Workers: 1, QueueSize: 2, Reconcile: 5 * time.Millisecond, Tick: 5 * time.Millisecond}, nil)
	<-started
	time.Sleep(20 * time.Millisecond)
	hold.Address = "hold"
	next.Address = "new"
	store.set(hold, next)
	time.Sleep(20 * time.Millisecond)
	close(release)
	waitUntil(t, func() bool {
		store.mu.Lock()
		defer store.mu.Unlock()
		for _, sample := range store.applied {
			if sample.TargetID == queued && sample.StatusClass == "new" {
				return true
			}
		}
		return false
	})
	store.mu.Lock()
	defer store.mu.Unlock()
	for _, sample := range store.applied {
		if sample.TargetID == queued && sample.StatusClass == "old" {
			t.Fatal("stale configuration was applied")
		}
	}
	cancel()
}

func TestCancelExits(t *testing.T) {
	store := &fakeStore{}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		run(ctx, store, nil, nil, Config{Workers: 1, QueueSize: 1, Reconcile: time.Hour, Tick: time.Hour}, nil)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("cancel did not stop the engine")
	}
}

func waitUntil(t *testing.T, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition was not met")
}
