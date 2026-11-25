package bootstrap

import (
	"context"
	"errors"
	"net/http"
)

// 生命周期函数：取消顺序固化——worker 先停队列/关 sender/client，api 先停 gRPC 再 Shutdown，
// 池关闭单点化（每退出路径恰一次）。

// RunAPI 并行监听 HTTP 与 gRPC，并在 ctx 结束或角色错误时优雅退出。
func RunAPI(ctx context.Context, api *API, sh *Shared) error {
	errCh := make(chan error, 2)
	go func() {
		if err := api.Server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()
	if api.GRPCServer != nil {
		go func() {
			if err := api.GRPCServer.Serve(); err != nil {
				errCh <- err
			}
		}()
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), sh.Cfg.HTTPShutdownTimeout)
	defer cancel()

	select {
	case err := <-errCh:
		if api.GRPCServer != nil {
			api.GRPCServer.GracefulStop(shutdownCtx)
		}
		sh.Pool.Close()
		return err
	case <-ctx.Done():
		if api.GRPCServer != nil {
			api.GRPCServer.GracefulStop(shutdownCtx)
		}
		err := api.Server.Shutdown(shutdownCtx)
		sh.Pool.Close()
		return err
	}
}

// RunWorker 启动后台组件并在 ctx 结束时逆序停止。
func RunWorker(ctx context.Context, w *Worker, sh *Shared) error {
	if w.Publisher != nil {
		go func() { _ = w.Publisher.Run(ctx) }()
	}
	if w.Consumer != nil {
		go func() { _ = w.Consumer.Run(ctx) }()
	}
	if w.TaskConsumer != nil {
		go func() { _ = w.TaskConsumer.Run(ctx) }()
	}
	if w.MonitorConsumer != nil {
		go func() { _ = w.MonitorConsumer.Run(ctx) }()
	}
	if w.Reconciler != nil {
		go w.Reconciler.Run(ctx)
	}
	if w.Probe != nil {
		go func() { _ = w.Probe.Run(ctx) }()
	}

	<-ctx.Done()
	stopWorker(w)
	sh.Pool.Close()
	return nil
}

func stopWorker(w *Worker) {
	if w.MonitorConsumer != nil {
		_ = w.MonitorConsumer.Close()
	}
	if w.TaskConsumer != nil {
		_ = w.TaskConsumer.Close()
	}
	if w.Consumer != nil {
		_ = w.Consumer.Close()
	}
	if w.Sender != nil {
		_ = w.Sender.Close()
	}
	if w.AgentClient != nil {
		_ = w.AgentClient.Close()
	}
}
