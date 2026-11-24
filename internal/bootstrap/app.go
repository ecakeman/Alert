package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ecakeman/alert/internal/application/health"
	appmonitor "github.com/ecakeman/alert/internal/application/monitor"
	appoutbox "github.com/ecakeman/alert/internal/application/outbox"
	portmonitor "github.com/ecakeman/alert/internal/application/port/monitor"
	"github.com/ecakeman/alert/internal/config"
	"github.com/ecakeman/alert/internal/infrastructure/messaging"
	"github.com/ecakeman/alert/internal/infrastructure/persistence/migrate"
	"github.com/ecakeman/alert/internal/infrastructure/persistence/postgres"
	"github.com/ecakeman/alert/internal/infrastructure/probe"
	"github.com/ecakeman/alert/internal/transport/http/handler"
	"github.com/ecakeman/alert/internal/transport/http/router"
)

// App 负责装配 HTTP 服务与真实依赖并管理生命周期。
type App struct {
	server      *http.Server
	cfg         config.Config
	pool        *pgxpool.Pool
	sender      *messaging.Sender
	publisher   *appoutbox.Publisher
	consumer    *messaging.Consumer
	probeEngine *probe.Engine
}

// New 是唯一的运行时装配入口，四步编排：
// NewShared（公共资源束）→ NewRepos（仓储实例集）→ NewUseCases（用例集）→ 组装 probe/handler/路由/outbox/http.Server。
func New(ctx context.Context, cfg config.Config) (*App, error) {
	sh, err := NewShared(ctx, cfg)
	if err != nil {
		return nil, err
	}
	repos := NewRepos(sh)
	ucs := NewUseCases(sh, repos)

	probeEngine := probe.NewEngine(probe.EngineDeps{
		Source: ucs.Monitor,
		HTTP:   probe.NewHTTPProber(), TCP: probe.NewTCPProber(),
		Handler: func(ctx context.Context, target portmonitor.TargetSnapshot, smp probe.Sample) error {
			_, err := ucs.Monitor.ApplySample(ctx, target.ServiceID, target.TargetID, target.ConfigGeneration, appmonitor.Sample{
				CheckStatus: smp.CheckStatus, StatusClass: smp.StatusClass, ErrorClass: smp.ErrorClass,
				LatencyMS: smp.LatencyMS, StateFingerprint: smp.StateFingerprint,
			})
			return err
		},
		Workers: cfg.ProbeWorkers, Capacity: cfg.ProbeQueueCapacity, Clock: sh.Clock, ScanEvery: time.Second,
	})

	healthH := handler.NewHealth(health.NewService(
		postgres.NewPGChecker(sh.Pool, cfg.ReadyzTimeout),
		postgres.NewMigrationChecker(sh.Pool, cfg.ReadyzTimeout),
	))
	engine := router.New(router.Deps{
		Health: healthH, Service: handler.NewService(ucs.Service),
		Target:    handler.NewTarget(ucs.Service),
		Dashboard: handler.NewDashboard(ucs.Dashboard),
		Episode:   handler.NewEpisode(ucs.Episode),
		Timeout:   cfg.HTTPReadTimeout,
	})

	var sender *messaging.Sender
	var publisher *appoutbox.Publisher
	var consumer *messaging.Consumer
	if cfg.OutboxPublisherEnabled {
		brokers := splitBrokers(cfg.KafkaBrokers)
		sender = messaging.NewSender(brokers, cfg.KafkaTopic)
		publisher = appoutbox.New(appoutbox.Deps{
			UOW: sh.UOW, Outbox: repos.Outbox, Sender: sender, Clock: sh.Clock,
			Owner: "system-core", Batch: cfg.OutboxBatch,
			ClaimLease: cfg.OutboxClaimLease, PollInterval: cfg.OutboxPollInterval,
		})
		consumer = messaging.NewConsumer(brokers, cfg.KafkaTopic, "system.audit.v1",
			func(ctx context.Context, body []byte) error {
				ev, err := messaging.Decode(body)
				if err != nil {
					return err
				}
				return ucs.AuditConsumer.Handle(ctx, ev)
			})
	}

	srv := &http.Server{
		Addr:         cfg.HTTPAddr,
		Handler:      engine,
		ReadTimeout:  cfg.HTTPReadTimeout,
		WriteTimeout: cfg.HTTPWriteTimeout,
		IdleTimeout:  cfg.HTTPIdleTimeout,
	}
	return &App{server: srv, cfg: cfg, pool: sh.Pool, sender: sender, publisher: publisher, consumer: consumer, probeEngine: probeEngine}, nil
}

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
	if a.publisher != nil {
		go func() { _ = a.publisher.Run(ctx) }()
	}
	if a.consumer != nil {
		go func() { _ = a.consumer.Run(ctx) }()
	}
	if a.probeEngine != nil {
		go func() { _ = a.probeEngine.Run(ctx) }()
	}

	errCh := make(chan error, 1)
	go func() {
		if err := a.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		a.closeBackground()
		a.pool.Close()
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), a.cfg.HTTPShutdownTimeout)
		defer cancel()
		err := a.server.Shutdown(shutdownCtx)
		a.closeBackground()
		a.pool.Close()
		return err
	}
}

func (a *App) closeBackground() {
	if a.consumer != nil {
		_ = a.consumer.Close()
	}
	if a.sender != nil {
		_ = a.sender.Close()
	}
}
