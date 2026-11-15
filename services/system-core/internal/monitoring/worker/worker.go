package worker

import (
	"context"
	"sync"

	"github.com/ecakeman/alertflow/services/system-core/internal/monitoring/queue"
)

// Run 启动固定数量的 worker。ctx 取消后，当前探测返回，函数等待全部退出。
func Run(ctx context.Context, workers int, jobs *queue.Queue, handle func(queue.Job)) {
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
					handle(job)
				}
			}
		}()
	}
	wg.Wait()
}
