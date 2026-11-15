package queue

import (
	"sync"

	"github.com/ecakeman/alertflow/services/system-core/internal/system"
	"github.com/google/uuid"
)

// Job 是一次已接受的探测。
type Job struct {
	Target     system.DueTarget
	Generation uint64
}

// Queue 是有界队列。同一目标在排队或执行期间不能再次进入。
type Queue struct {
	ch   chan Job
	mu   sync.Mutex
	busy map[uuid.UUID]struct{}
}

func New(size int) *Queue {
	if size <= 0 {
		size = 1
	}
	return &Queue{ch: make(chan Job, size), busy: map[uuid.UUID]struct{}{}}
}

func (q *Queue) Chan() <-chan Job { return q.ch }

// TryEnqueue 在队列满或目标已占用时拒绝。拒绝时调用方必须把目标留在调度堆里。
func (q *Queue) TryEnqueue(job Job) (accepted bool, reason string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if _, ok := q.busy[job.Target.TargetID]; ok {
		return false, "busy"
	}
	if len(q.ch) == cap(q.ch) {
		return false, "full"
	}
	q.busy[job.Target.TargetID] = struct{}{}
	q.ch <- job
	return true, ""
}

// Done 释放目标，使它可以再次调度。
func (q *Queue) Done(id uuid.UUID) {
	q.mu.Lock()
	delete(q.busy, id)
	q.mu.Unlock()
}
