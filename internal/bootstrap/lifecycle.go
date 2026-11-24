package bootstrap

import (
	"context"
	"errors"
	"net/http"
)

// 生命周期函数：取消顺序固化——worker 先停队列/关 sender/client，api 再 Shutdown，
// 池关闭单点化（每退出路径恰一次）。RunCombined 为过渡组合专用，S2.7 删除。

// RunAPI 启动 HTTP 并在 ctx 结束或角色错误时优雅退出。
func RunAPI(ctx context.Context, api *API, sh *Shared) error {
	errCh := make(chan error, 1)
	go func() {
		if err := api.Server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		sh.Pool.Close()
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), sh.Cfg.HTTPShutdownTimeout)
		defer cancel()
		err := api.Server.Shutdown(shutdownCtx)
		sh.Pool.Close()
		return err
	}
}

// RunWorker 启动后台组件（空态跳过）并在 ctx 结束时先停队列/关 client 再关池。
func RunWorker(ctx context.Context, w *Worker, sh *Shared) error {
	if w.Publisher != nil {
		go func() { _ = w.Publisher.Run(ctx) }()
	}
	if w.Consumer != nil {
		go func() { _ = w.Consumer.Run(ctx) }()
	}
	if w.Probe != nil {
		go func() { _ = w.Probe.Run(ctx) }()
	}

	<-ctx.Done()
	if w.Consumer != nil {
		_ = w.Consumer.Close()
	}
	if w.Sender != nil {
		_ = w.Sender.Close()
	}
	sh.Pool.Close()
	return nil
}

// RunCombined 过渡组合专用（S2.7 随分进程删除）：单进程内 worker 先启动、HTTP 后监听；
// ctx.Done 时依次执行 worker 停队列 + sender/client 关闭 → api.Shutdown → pool.Close。
func RunCombined(ctx context.Context, api *API, w *Worker, sh *Shared) error {
	if w.Publisher != nil {
		go func() { _ = w.Publisher.Run(ctx) }()
	}
	if w.Consumer != nil {
		go func() { _ = w.Consumer.Run(ctx) }()
	}
	if w.Probe != nil {
		go func() { _ = w.Probe.Run(ctx) }()
	}

	errCh := make(chan error, 1)
	go func() {
		if err := api.Server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		stopWorker(w)
		sh.Pool.Close()
		return err
	case <-ctx.Done():
		stopWorker(w)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), sh.Cfg.HTTPShutdownTimeout)
		defer cancel()
		err := api.Server.Shutdown(shutdownCtx)
		sh.Pool.Close()
		return err
	}
}

func stopWorker(w *Worker) {
	if w.Consumer != nil {
		_ = w.Consumer.Close()
	}
	if w.Sender != nil {
		_ = w.Sender.Close()
	}
}
