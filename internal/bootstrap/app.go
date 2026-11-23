package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ecakeman/alert/internal/application/health"
	"github.com/ecakeman/alert/internal/config"
	"github.com/ecakeman/alert/internal/infrastructure/persistence/migrate"
	"github.com/ecakeman/alert/internal/infrastructure/persistence/postgres"
	"github.com/ecakeman/alert/internal/transport/http/handler"
	"github.com/ecakeman/alert/internal/transport/http/router"
)

// App 负责装配 HTTP 服务与真实依赖并管理生命周期。
type App struct {
	server *http.Server
	cfg    config.Config
	pool   *pgxpool.Pool
}

// New 是唯一的运行时装配入口：配置校验 → pgx 池 → 可选迁移 → 双 checker → 路由 → HTTP。
func New(ctx context.Context, cfg config.Config) (*App, error) {
	if err := cfg.ValidateRuntime(); err != nil {
		return nil, err
	}

	pool, err := postgres.NewPool(ctx, cfg)
	if err != nil {
		return nil, err
	}

	if cfg.MigrateOnStart {
		res, err := migrate.Up(ctx, pool)
		if err != nil {
			pool.Close()
			return nil, err
		}
		if len(res.Conflicts) > 0 {
			pool.Close()
			return nil, fmt.Errorf("migration conflicts: %v", res.Conflicts)
		}
	}

	h := handler.NewHealth(health.NewService(
		postgres.NewPGChecker(pool, cfg.ReadyzTimeout),
		postgres.NewMigrationChecker(pool, cfg.ReadyzTimeout),
	))
	engine := router.New(router.Deps{Health: h, Timeout: cfg.HTTPReadTimeout})

	srv := &http.Server{
		Addr:         cfg.HTTPAddr,
		Handler:      engine,
		ReadTimeout:  cfg.HTTPReadTimeout,
		WriteTimeout: cfg.HTTPWriteTimeout,
		IdleTimeout:  cfg.HTTPIdleTimeout,
	}
	return &App{server: srv, cfg: cfg, pool: pool}, nil
}

// RunMigrate 只执行 forward-only 迁移后退出，不启动 HTTP。
func RunMigrate(ctx context.Context, cfg config.Config) error {
	if err := cfg.ValidateRuntime(); err != nil {
		return err
	}
	pool, err := postgres.NewPool(ctx, cfg)
	if err != nil {
		return err
	}
	defer pool.Close()

	res, err := migrate.Up(ctx, pool)
	if err != nil {
		return err
	}
	if len(res.Conflicts) > 0 {
		return fmt.Errorf("migration conflicts: %v", res.Conflicts)
	}
	fmt.Printf("applied=%d skipped=%d conflicts=%d\n", len(res.Applied), len(res.Skipped), len(res.Conflicts))
	return nil
}

// Run 启动服务并在 ctx 结束时优雅退出；所有路径都关闭连接池。
func (a *App) Run(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		if err := a.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		a.pool.Close()
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), a.cfg.HTTPShutdownTimeout)
		defer cancel()
		err := a.server.Shutdown(shutdownCtx)
		a.pool.Close()
		return err
	}
}