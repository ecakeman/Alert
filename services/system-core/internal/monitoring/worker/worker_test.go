package worker

import (
	"context"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ecakeman/alertflow/services/system-core/internal/monitoring/queue"
	"github.com/ecakeman/alertflow/services/system-core/internal/system"
	"github.com/google/uuid"
)

func testJob(n int) queue.Job {
	return queue.Job{
		Target: system.DueTarget{TargetID: uuid.New(), TargetType: "HTTP", Address: "http://job" + strconv.Itoa(n)},
	}
}

// TestWorkerSurvivesPanic 是 T3：句柄 panic 之后 worker 必须仍然处理后续 job。
// 原实现没有 recover，panic 会直接杀死 goroutine；4 个 worker 各 panic 一次
// 就全部消失，调度永久停摆且没有任何输出。
func TestWorkerSurvivesPanic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	jobs := queue.New(8)
	var handled atomic.Int32
	var panics atomic.Int32
	done := make(chan struct{})

	go func() {
		RunWithOptions(ctx, 1, jobs, func(job queue.Job) {
			n := handled.Add(1)
			// 第一个 job panic，之后必须正常处理。
			if n == 1 {
				panic("boom")
			}
			if n >= 3 {
				select {
				case <-done:
				default:
					close(done)
				}
			}
		}, Options{OnPanic: func(queue.Job, any) { panics.Add(1) }})
	}()

	// 塞 3 个 job：第 1 个 panic，第 2、3 个用来证明 worker 还活着。
	go func() {
		for i := 1; i <= 3; i++ {
			jobs.TryEnqueue(testJob(i))
			// Done 在这里不重要，worker 只关心句柄是否被调用。
			time.Sleep(time.Millisecond)
		}
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("worker stopped processing after a panic")
	}
	if got := panics.Load(); got != 1 {
		t.Fatalf("OnPanic calls = %d, want 1", got)
	}
	cancel()
}

// TestWorkerNeverDiesAcrossManyPanics：连续大量 panic 后 worker 仍有吞吐，
// 验证 recover 后是 continue 而不是退出。
func TestWorkerNeverDiesAcrossManyPanics(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	jobs := queue.New(4)
	var handled atomic.Int32
	var panics atomic.Int32
	enough := make(chan struct{})
	var once sync.Once

	go RunWithOptions(ctx, 2, jobs, func(queue.Job) {
		n := handled.Add(1)
		if n <= 10 {
			panic("boom")
		}
		if n >= 12 {
			once.Do(func() { close(enough) })
		}
	}, Options{OnPanic: func(queue.Job, any) { panics.Add(1) }})

	feeder := make(chan struct{})
	go func() {
		defer close(feeder)
		for i := 0; i < 12; i++ {
			for {
				accepted, _ := jobs.TryEnqueue(testJob(i))
				if accepted {
					break
				}
				time.Sleep(time.Millisecond)
			}
			time.Sleep(time.Millisecond)
		}
	}()

	select {
	case <-enough:
	case <-time.After(3 * time.Second):
		t.Fatalf("worker died: handled=%d panics=%d after 10 panics", handled.Load(), panics.Load())
	}
	if got := panics.Load(); got != 10 {
		t.Fatalf("OnPanic calls = %d, want 10", got)
	}
	cancel()
}

// TestRunWithoutOptionsStillRecovers：公开 Run 不传 OnPanic 也不能因为 panic 而丢 worker。
func TestRunWithoutOptionsStillRecovers(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	jobs := queue.New(4)
	handled := make(chan struct{})
	var n atomic.Int32
	go Run(ctx, 1, jobs, func(queue.Job) {
		if n.Add(1) == 1 {
			panic("boom")
		}
		select {
		case <-handled:
		default:
			close(handled)
		}
	})
	go func() {
		for i := 0; i < 2; i++ {
			for {
				if ok, _ := jobs.TryEnqueue(testJob(i)); ok {
					break
				}
				time.Sleep(time.Millisecond)
			}
			time.Sleep(time.Millisecond)
		}
	}()
	select {
	case <-handled:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not survive a panic with the default Options")
	}
	cancel()
}
