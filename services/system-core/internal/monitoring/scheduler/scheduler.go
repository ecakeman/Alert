package scheduler

import (
	"container/heap"
	"encoding/json"
	"sync"
	"time"

	"github.com/ecakeman/alertflow/services/system-core/internal/system"
	"github.com/google/uuid"
)

type Item struct {
	Target     system.DueTarget
	Due        time.Time
	Generation uint64
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
	*h = append(*h, item)
}
func (h *minHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	*h = old[:n-1]
	item.index = -1
	return item
}

// Scheduler 按 next_due 取到期目标，并记住配置代次。
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
	seen := map[uuid.UUID]bool{}
	for _, target := range targets {
		seen[target.TargetID] = true
		fp := fingerprint(target)
		item, ok := s.byID[target.TargetID]
		if !ok {
			item = &Item{Target: target, Due: now, Generation: 1, fp: fp}
			s.byID[target.TargetID] = item
			s.push(item)
			continue
		}
		item.Target = target
		item.gone = false
		if item.fp != fp {
			item.fp = fp
			item.Generation++
		}
		if !item.inHeap {
			item.Due = now
			s.push(item)
		}
	}
	for id, item := range s.byID {
		if seen[id] {
			continue
		}
		item.gone = true
		if item.inHeap {
			heap.Remove(&s.items, item.index)
			item.inHeap = false
		}
		delete(s.byID, id)
	}
}

func (s *Scheduler) push(item *Item) {
	item.inHeap = true
	heap.Push(&s.items, item)
}

// PopDue 取出一个已到期且仍有效的目标。
func (s *Scheduler) PopDue(now time.Time) (Item, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for s.items.Len() > 0 {
		item := s.items[0]
		if item.gone {
			heap.Pop(&s.items)
			item.inHeap = false
			continue
		}
		if item.Due.After(now) {
			return Item{}, false
		}
		heap.Pop(&s.items)
		item.inHeap = false
		return *item, true
	}
	return Item{}, false
}

// Defer 把未能入队的目标留在堆里。
func (s *Scheduler) Defer(id uuid.UUID, due time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item := s.byID[id]
	if item == nil || item.gone {
		return
	}
	item.Due = due
	if item.inHeap {
		heap.Fix(&s.items, item.index)
		return
	}
	s.push(item)
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
