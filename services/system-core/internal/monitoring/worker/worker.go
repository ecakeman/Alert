package worker

import (
	"context"
	"sync"

	"github.com/ecakeman/alertflow/services/system-core/internal/monitoring/queue"
)

// Options 是可选的 worker 配置。零值表示使用默认。
type Options struct {
	// OnPanic 在某个 job 的句柄 panic 后被调用。nil 时 panic 被静默吞掉
	// （绝不能让 worker goroutine 因为一次探测 panic 而永久退出）。
	OnPanic func(job queue.Job, recovered any)
}

// Run 启动固定数量的 worker。ctx 取消后，当前探测返回，函数等待全部退出。
func Run(ctx context.Context, workers int, jobs *queue.Queue, handle func(queue.Job)) {
	RunWithOptions(ctx, workers, jobs, handle, Options{})
}

// RunWithOptions 与 Run 相同，但允许注入 OnPanic 回调。
//
// 每个 worker goroutine 都在单次 job 的边界上 recover：句柄 panic 只影响
// 这一次探测，worker 继续处理后续 job。原实现里 4 个 worker 各 panic 一次
// 就会全部静默死亡，调度永久停摆且没有任何 panic 输出。
func RunWithOptions(ctx context.Context, workers int, jobs *queue.Queue, handle func(queue.Job), opts Options) {
	if workers <= 0 {
		workers = 4
	}
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case job := <-jobs.Chan():
					safeHandle(job, handle, opts.OnPanic)
				}
			}
		}()
	}
	wg.Wait()
}

// safeHandle 执行一次 job，panic 不会逃出这个函数，因此不会杀死 worker 循环。
func safeHandle(job queue.Job, handle func(queue.Job), onPanic func(queue.Job, any)) {
	defer func() {
		if r := recover(); r != nil {
			if onPanic != nil {
				onPanic(job, r)
			}
		}
	}()
	handle(job)
}
