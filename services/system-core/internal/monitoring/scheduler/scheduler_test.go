package scheduler

import (
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/ecakeman/alertflow/services/system-core/internal/system"
	"github.com/google/uuid"
)

func dueTarget(id uuid.UUID, address string, interval time.Duration) system.DueTarget {
	return system.DueTarget{
		TargetID:   id,
		TargetType: "HTTP",
		Address:    address,
		Interval:   interval,
		Timeout:    time.Second,
	}
}

// requireHeapOrder 断言堆序不变量：任意父节点都早于子节点，并且每个元素的
// index 与它在切片里的位置一致。堆序被破坏时 PopDue 会取不到最早到期元素，
// 表现为采样永久停摆（不会有 panic），所以必须显式检查。
func requireHeapOrder(t *testing.T, s *Scheduler) {
	t.Helper()
	for i := range s.items {
		item := s.items[i]
		if !item.inHeap {
			t.Fatalf("item %s at %d has inHeap=false", item.Target.TargetID, i)
		}
		if item.index != i {
			t.Fatalf("item %s index=%d but sits at %d", item.Target.TargetID, item.index, i)
		}
		if i == 0 {
			continue
		}
		parent := (i - 1) / 2
		if s.items[parent].Due.After(item.Due) {
			t.Fatalf("heap order broken: parent[%d].Due=%s after child[%d].Due=%s",
				parent, s.items[parent].Due.Format(time.RFC3339Nano),
				i, item.Due.Format(time.RFC3339Nano))
		}
	}
	if n := countInHeap(s); n != s.items.Len() {
		t.Fatalf("inHeap count=%d but heap len=%d", n, s.items.Len())
	}
}

func countInHeap(s *Scheduler) int {
	n := 0
	for _, item := range s.byID {
		if item.inHeap {
			n++
		}
	}
	return n
}

// TestSchedulerConcurrentSyncPopDeferKeepsHeapOrder 是 T1：
// 多个 goroutine 反复 Sync / PopDue / Defer，PopDue 取到的元素一定 ≤ now，
// 并且不丢目标、堆序始终成立。
func TestSchedulerConcurrentSyncPopDeferKeepsHeapOrder(t *testing.T) {
	const targetCount = 64
	ids := make([]uuid.UUID, targetCount)
	targets := make([]system.DueTarget, targetCount)
	for i := range ids {
		ids[i] = uuid.New()
		targets[i] = dueTarget(ids[i], fmt.Sprintf("http://t%d", i), time.Second)
	}
	s := New()
	s.Sync(targets, time.Now())

	stop := make(chan struct{})
	var wg sync.WaitGroup

	// Sync 侧：像 engine 那样每 5ms 重放一次快照，中途改地址触发代次变化。
	// 故意不按原顺序给快照，模拟数据库返回顺序抖动。
	wg.Add(1)
	go func() {
		defer wg.Done()
		rnd := rand.New(rand.NewSource(1))
		for {
			select {
			case <-stop:
				return
			default:
			}
			snapshot := append([]system.DueTarget(nil), targets...)
			rnd.Shuffle(len(snapshot), func(i, j int) {
				snapshot[i], snapshot[j] = snapshot[j], snapshot[i]
			})
			// 每轮把其中一个目标的地址翻转，制造真实的配置/代次变化。
			victim := rnd.Intn(len(snapshot))
			snapshot[victim].Address = snapshot[victim].Address + "#"
			s.Sync(snapshot, time.Now())
			time.Sleep(time.Millisecond)
		}
	}()

	// worker 侧：PopDue → 立刻 Defer 回堆，模拟探测结束后归还。
	// 关键断言：PopDue 返回的 Due 必须 ≤ 传入的 now。
	const workerGoroutines = 4
	var popped [workerGoroutines]int
	for w := 0; w < workerGoroutines; w++ {
		w := w
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				now := time.Now()
				item, ok := s.PopDue(now)
				if !ok {
					time.Sleep(time.Millisecond)
					continue
				}
				if item.Due.After(now) {
					t.Errorf("PopDue returned Due=%s after now=%s (future item popped)",
						item.Due.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
					return
				}
				popped[w]++
				// 归还：间隔 10ms 后再次到期，让循环持续转动。
				s.Defer(item.Target.TargetID, time.Now().Add(10*time.Millisecond), item.Version)
			}
		}()
	}

	// 主 goroutine 周期性检查堆不变量。检查要拿锁，所以直接复用内部状态。
	deadline := time.After(50 * time.Millisecond)
	for {
		select {
		case <-deadline:
			close(stop)
			wg.Wait()
			s.mu.Lock()
			requireHeapOrder(t, s)
			s.mu.Unlock()
			total := 0
			for _, n := range popped {
				total += n
			}
			if total == 0 {
				t.Fatal("no target was scheduled in 50ms: scheduler stalled")
			}
			// 目标没有被 Sync 删除，byID 必须仍然齐全。
			s.mu.Lock()
			if len(s.byID) != targetCount {
				t.Fatalf("byID=%d want %d (targets were dropped)", len(s.byID), targetCount)
			}
			// 每个目标的 Due 都必须是有限的、非零的时刻。
			for _, id := range ids {
				item := s.byID[id]
				if item == nil {
					t.Fatalf("target %s disappeared", id)
				}
				if item.Due.IsZero() {
					t.Fatalf("target %s has zero Due", id)
				}
			}
			s.mu.Unlock()
			return
		case <-time.After(2 * time.Millisecond):
			s.mu.Lock()
			requireHeapOrder(t, s)
			s.mu.Unlock()
		}
	}
}

// TestSchedulerSyncRaceKeepsInvariants 是 T2：
// 1000 个目标，大量 Sync 并发交织着 PopDue/Defer 之后，堆仍有序，
// s.items.Len() 与 inHeap 的 Item 数一致。
func TestSchedulerSyncRaceKeepsInvariants(t *testing.T) {
	const n = 1000
	ids := make([]uuid.UUID, n)
	targets := make([]system.DueTarget, n)
	for i := range ids {
		ids[i] = uuid.New()
		targets[i] = dueTarget(ids[i], fmt.Sprintf("http://n%d", i), 250*time.Millisecond)
	}
	s := New()
	s.Sync(targets, time.Now())

	var wg sync.WaitGroup
	// 8 个 Sync goroutine 反复重放，每个都在自己的副本里乱序并改动若干地址。
	for g := 0; g < 8; g++ {
		g := g
		wg.Add(1)
		go func() {
			defer wg.Done()
			rnd := rand.New(rand.NewSource(int64(g)))
			for round := 0; round < 20; round++ {
				snapshot := append([]system.DueTarget(nil), targets...)
				for k := 0; k < 50; k++ {
					i := rnd.Intn(n)
					snapshot[i].Address = fmt.Sprintf("http://n%d/v%d", i, round)
				}
				s.Sync(snapshot, time.Now())
			}
		}()
	}
	// 4 个消费 goroutine：出堆再归还，制造「快照 + 配置变化」的交织。
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for round := 0; round < 400; round++ {
				now := time.Now()
				item, ok := s.PopDue(now)
				if !ok {
					time.Sleep(100 * time.Microsecond)
					continue
				}
				if item.Due.After(now) {
					t.Errorf("PopDue returned future Due %s > now %s",
						item.Due.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
					return
				}
				s.Defer(item.Target.TargetID, time.Now().Add(time.Millisecond), item.Version)
			}
		}()
	}
	wg.Wait()

	// 把所有目标重新 Sync 回来（上面有些快照删掉过）并检查不变量。
	s.Sync(targets, time.Now())
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := heapCheck(s.items); err != "" {
		t.Fatal(err)
	}
	if s.items.Len() != countInHeap(s) {
		t.Fatalf("heap len=%d but inHeap count=%d", s.items.Len(), countInHeap(s))
	}
	if len(s.byID) != n {
		t.Fatalf("byID=%d want %d", len(s.byID), n)
	}
	// 消费 goroutine 最后可能把某个目标 pop 到手就退出了（在途），
	// Sync 有意不抢跑在途目标，所以允许最多 4 个不在堆里 —— 但它们必须是
	// 干净的「在途」状态：index 已重置为 -1，没有残留的堆位置。
	inflight := 0
	for _, item := range s.byID {
		if item.inHeap {
			continue
		}
		inflight++
		if item.index != -1 {
			t.Fatalf("inflight target %s keeps stale index %d", item.Target.TargetID, item.index)
		}
	}
	if inflight > 4 {
		t.Fatalf("inflight=%d, want at most 4", inflight)
	}
	// 每个堆内元素的 index 都必须指回自己。
	for i, item := range s.items {
		if item.index != i {
			t.Fatalf("target %s index=%d but sits at %d", item.Target.TargetID, item.index, i)
		}
	}
}

func heapCheck(h minHeap) string {
	for i := range h {
		if i > 0 {
			parent := (i - 1) / 2
			if h[parent].Due.After(h[i].Due) {
				return fmt.Sprintf("heap order broken at %d: parent=%s child=%s",
					i, h[parent].Due.Format(time.RFC3339Nano), h[i].Due.Format(time.RFC3339Nano))
			}
		}
		if h[i].index != i {
			return fmt.Sprintf("index mismatch at %d: item.index=%d", i, h[i].index)
		}
	}
	return ""
}

// TestDeferRejectsStaleVersion 直接覆盖 P0 的直接触发条件：
// 目标出堆执行期间被 Sync 重新入堆/改配置，旧快照的 Defer 必须被丢弃，
// 不能把已经失效的 Due 原地 heap.Fix 到错误位置。
func TestDeferRejectsStaleVersion(t *testing.T) {
	id := uuid.New()
	s := New()
	original := dueTarget(id, "http://stale", time.Second)
	s.Sync([]system.DueTarget{original}, time.Now())

	item, ok := s.PopDue(time.Now().Add(time.Second))
	if !ok {
		t.Fatal("target was not due")
	}
	// 执行期间配置变了：地址变更 → 代次与版本都推进。
	changed := original
	changed.Address = "http://changed"
	s.Sync([]system.DueTarget{changed}, time.Now())

	// 旧快照归还，必须被拒绝。
	s.Defer(id, time.Now().Add(time.Hour), item.Version)

	s.mu.Lock()
	defer s.mu.Unlock()
	requireHeapOrder(t, s)
	current := s.byID[id]
	if current == nil {
		t.Fatal("target disappeared")
	}
	if current.Due.Equal(time.Now().Add(time.Hour).Round(0)) {
		t.Fatal("stale Defer was applied")
	}
	if current.Target.Address != "http://changed" {
		t.Fatalf("address = %s, want changed config", current.Target.Address)
	}
	if current.Generation != item.Generation+1 {
		t.Fatalf("generation = %d, want %d", current.Generation, item.Generation+1)
	}
}

// TestDeferAppliesCurrentVersion 覆盖正常路径：版本匹配时 Defer 必须真正回堆。
func TestDeferAppliesCurrentVersion(t *testing.T) {
	id := uuid.New()
	s := New()
	s.Sync([]system.DueTarget{dueTarget(id, "http://ok", time.Second)}, time.Now())
	item, ok := s.PopDue(time.Now().Add(time.Second))
	if !ok {
		t.Fatal("target was not due")
	}
	want := time.Now().Add(500 * time.Millisecond)
	s.Defer(id, want, item.Version)

	s.mu.Lock()
	defer s.mu.Unlock()
	requireHeapOrder(t, s)
	current := s.byID[id]
	if !current.inHeap {
		t.Fatal("current-version Defer did not put the target back on the heap")
	}
	if !current.Due.Equal(want) {
		t.Fatalf("Due = %s, want %s", current.Due.Format(time.RFC3339Nano), want.Format(time.RFC3339Nano))
	}
}

// TestRemoveThenPushResetsIndex 覆盖 Remove 后 index 失效：删掉元素再重新 Sync 回来，
// index 必须由 heap 重新维护，不能沿用旧值。
func TestRemoveThenPushResetsIndex(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	s := New()
	ta, tb := dueTarget(a, "a", time.Second), dueTarget(b, "b", time.Second)
	s.Sync([]system.DueTarget{ta, tb}, time.Now())
	// 删除 a（heap.Remove 会移动别的元素、改写它们的 index）。
	s.Sync([]system.DueTarget{tb}, time.Now())
	// 再把 a 加回来。
	s.Sync([]system.DueTarget{ta, tb}, time.Now())

	s.mu.Lock()
	defer s.mu.Unlock()
	requireHeapOrder(t, s)
	if len(s.items) != 2 {
		t.Fatalf("heap len = %d, want 2", len(s.items))
	}
}
