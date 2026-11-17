package scheduler

import (
	"container/heap"
	"encoding/json"
	"sync"
	"time"

	"github.com/ecakeman/alertflow/services/system-core/internal/system"
	"github.com/google/uuid"
)

// Item 是堆里（或正在执行中）的一个目标。version 在每次配置变化、
// 代次变化或从堆里取走时递增，用来把「取时快照」和「归还时的状态」对上。
type Item struct {
	Target     system.DueTarget
	Due        time.Time
	Generation uint64
	Version    uint64
	index      int
	inHeap     bool
	gone       bool
	fp         string
}

type minHeap []*Item

func (h minHeap) Len() int           { return len(h) }
func (h minHeap) Less(i, j int) bool { return h[i].Due.Before(h[j].Due) }
func (h minHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}
func (h *minHeap) Push(x any) {
	item := x.(*Item)
	item.index = len(*h)
	item.inHeap = true
	*h = append(*h, item)
}
func (h *minHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	*h = old[:n-1]
	item.index = -1
	item.inHeap = false
	return item
}

// Scheduler 按 next_due 取到期目标，并记住配置代次。
//
// 堆结构的不变量是「就地修改 Due 必须紧跟在修好 index 的 heap.Fix 之后」。
// 出堆的目标由 worker 持有，期间可能被 Sync 重新入堆、改配置或删除；
// 所以所有对 Item 的改动都只允许发生在 s.mu 内，并且都要让 Item.Version 递增，
// 归还时用 Version 校验，避免把已经作废的 Due 写回堆里打乱堆序。
type Scheduler struct {
	mu    sync.Mutex
	items minHeap
	byID  map[uuid.UUID]*Item
}

func New() *Scheduler {
	return &Scheduler{byID: map[uuid.UUID]*Item{}}
}

func fingerprint(target system.DueTarget) string {
	raw, _ := json.Marshal(struct {
		Type     string          `json:"type"`
		Address  string          `json:"address"`
		Config   json.RawMessage `json:"config"`
		Interval int64           `json:"interval"`
		Timeout  int64           `json:"timeout"`
	}{target.TargetType, target.Address, target.Config, int64(target.Interval), int64(target.Timeout)})
	return string(raw)
}

// Sync 用数据库快照修正堆。配置变化会增加代次，删除的目标不再调度。
func (s *Scheduler) Sync(targets []system.DueTarget, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.syncLocked(targets, now)
}

func (s *Scheduler) syncLocked(targets []system.DueTarget, now time.Time) {
	seen := map[uuid.UUID]bool{}
	for _, target := range targets {
		seen[target.TargetID] = true
		fp := fingerprint(target)
		item, ok := s.byID[target.TargetID]
		if !ok {
			item = &Item{Target: target, Due: now, Generation: 1, Version: 1, fp: fp}
			s.byID[target.TargetID] = item
			s.pushLocked(item)
			continue
		}
		item.Target = target
		item.gone = false
		if item.fp != fp {
			item.fp = fp
			item.Generation++
			// 配置变了，在途 worker 手上的快照作废，它归还时会因为 Version 不匹配被丢弃。
			item.Version++
			// 正因为旧快照会被丢弃，必须由 Sync 负责把目标重新放回堆里，
			// 否则没人归还它（队列那边 busy 会被 Done 释放，但调度堆里就永久缺了它）。
			// 直接用 now 入堆，等价于「配置变更后立即重采一次」。
			if !item.inHeap {
				item.Due = now
				s.pushLocked(item)
				continue
			}
		}
		if !item.inHeap {
			// 配置没变、目标在途：由它自己的 Defer 归还，这里不抢跑，
			// 否则同一目标会被重复入堆、worker 又被 busy 拒掉。
			continue
		}
		if item.Due.After(now) {
			item.Due = now
			heap.Fix(&s.items, item.index)
		}
	}
	for id, item := range s.byID {
		if seen[id] {
			continue
		}
		item.gone = true
		item.Version++
		if item.inHeap {
			s.removeLocked(item)
		}
		// 目标已删除，队列则交由 ListDueTargets 负责，本地状态直接丢弃。
		delete(s.byID, id)
	}
}

// pushLocked 入堆并重置 index（Remove 之后 Item 的 index 不可信）。
func (s *Scheduler) pushLocked(item *Item) {
	heap.Push(&s.items, item)
}

func (s *Scheduler) removeLocked(item *Item) {
	if !item.inHeap {
		return
	}
	heap.Remove(&s.items, item.index)
	item.index = -1
	item.inHeap = false
}

// PopDue 取出一个已到期且仍有效的目标。返回的 Item 是快照，Version 用于归还时校验。
func (s *Scheduler) PopDue(now time.Time) (Item, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.popDueLocked(now)
}

func (s *Scheduler) popDueLocked(now time.Time) (Item, bool) {
	for s.items.Len() > 0 {
		item := s.items[0]
		if item.gone {
			s.removeLocked(item)
			continue
		}
		if item.Due.After(now) {
			return Item{}, false
		}
		s.removeLocked(item)
		// 出堆即作废旧快照：worker 回来时要么 Version 对不上（有新配置/重新入堆），
		// 要么一定走一次完整的 pushLocked 而非原地 Fix。
		item.Version++
		return *item, true
	}
	return Item{}, false
}

// Defer 把未能入队的目标留在堆里。version 应当来自 PopDue 取到的 Item，
// 只有与当前 Item 的 Version 一致时才生效；否则说明期间配置变了或被重新调度，
// 这次 Due 已经过期，直接丢弃，由 Task 侧按新配置重新排。
func (s *Scheduler) Defer(id uuid.UUID, due time.Time, version uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deferLocked(id, due, version)
}

func (s *Scheduler) deferLocked(id uuid.UUID, due time.Time, version uint64) {
	item := s.byID[id]
	if item == nil || item.gone {
		return
	}
	if item.Version != version {
		return
	}
	item.Version++
	item.Due = due
	if item.inHeap {
		// 快照版本对得上却没有出堆，只可能是调用方没走 PopDue；先摘掉再加，
		// 保证 index 一定由 heap 自己维护，不会出现 index 指向别的元素。
		s.removeLocked(item)
	}
	s.pushLocked(item)
}

// Generation 返回目标当前配置代次。目标已删除时返回 0。
func (s *Scheduler) Generation(id uuid.UUID) uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	item := s.byID[id]
	if item == nil || item.gone {
		return 0
	}
	return item.Generation
}
