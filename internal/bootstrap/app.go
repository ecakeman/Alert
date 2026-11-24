package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	appdashboard "github.com/ecakeman/alert/internal/application/dashboard"
	appepisode "github.com/ecakeman/alert/internal/application/episode"
	"github.com/ecakeman/alert/internal/application/health"
	appmonitor "github.com/ecakeman/alert/internal/application/monitor"
	appoutbox "github.com/ecakeman/alert/internal/application/outbox"
	portmonitor "github.com/ecakeman/alert/internal/application/port/monitor"
	appservice "github.com/ecakeman/alert/internal/application/service"
	"github.com/ecakeman/alert/internal/config"
	"github.com/ecakeman/alert/internal/infrastructure"
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

// New 是唯一的运行时装配入口：配置校验 → pgx 池 → 可选迁移 → 用例 → handler → 路由 → HTTP。
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

	uow := postgres.NewUnitOfWork(pool)
	clk := infrastructure.RealClock{}
	ids := infrastructure.IDGen{}

	svcUC := appservice.New(appservice.Deps{
		UOW: uow, Services: postgres.NewServiceRepo(), Targets: postgres.NewTargetRepo(),
		Sources: postgres.NewSourceStateRepo(), Episodes: postgres.NewEpisodeRepo(),
		Outbox: postgres.NewOutboxRepo(), Clock: clk, IDs: ids,
	})
	dashUC := appdashboard.New(appdashboard.Deps{
		UOW: uow, Repo: postgres.NewDashboardRepo(), Clock: clk,
	})
	epUC := appepisode.New(appepisode.Deps{
		UOW: uow, Episodes: postgres.NewEpisodeRepo(), InputVersions: postgres.NewInputVersionRepo(),
		AppEvents: postgres.NewAppEventRepo(), MonitorPoints: postgres.NewMonitorPointRepo(),
		Sources: postgres.NewSourceStateRepo(), Services: postgres.NewServiceRepo(),
		Tasks: postgres.NewTaskRepo(), Artifacts: postgres.NewArtifactRepo(),
		Outbox: postgres.NewOutboxRepo(), Clock: clk, IDs: ids,
	})

	monitorUC := appmonitor.New(appmonitor.Deps{
		UOW: uow, Targets: postgres.NewTargetRepo(), MonitorPoints: postgres.NewMonitorPointRepo(),
		Sources: postgres.NewSourceStateRepo(), Episodes: postgres.NewEpisodeRepo(),
		Services: postgres.NewServiceRepo(),
		Driver:   episodeDriverAdapter{uc: epUC},
		Clock:    clk,
	})
	probeEngine := probe.NewEngine(probe.EngineDeps{
		Source: monitorUC,
		HTTP:   probe.NewHTTPProber(), TCP: probe.NewTCPProber(),
		Handler: func(ctx context.Context, target portmonitor.TargetSnapshot, smp probe.Sample) error {
			_, err := monitorUC.ApplySample(ctx, target.ServiceID, target.TargetID, target.ConfigGeneration, appmonitor.Sample{
				CheckStatus: smp.CheckStatus, StatusClass: smp.StatusClass, ErrorClass: smp.ErrorClass,
				LatencyMS: smp.LatencyMS, StateFingerprint: smp.StateFingerprint,
			})
			return err
		},
		Workers: cfg.ProbeWorkers, Capacity: cfg.ProbeQueueCapacity, Clock: clk, ScanEvery: time.Second,
	})

	healthH := handler.NewHealth(health.NewService(
		postgres.NewPGChecker(pool, cfg.ReadyzTimeout),
		postgres.NewMigrationChecker(pool, cfg.ReadyzTimeout),
	))
	engine := router.New(router.Deps{
		Health: healthH, Service: handler.NewService(svcUC),
		Target:    handler.NewTarget(svcUC),
		Dashboard: handler.NewDashboard(dashUC),
		Episode:   handler.NewEpisode(epUC),
		Timeout:   cfg.HTTPReadTimeout,
	})

	var sender *messaging.Sender
	var publisher *appoutbox.Publisher
	var consumer *messaging.Consumer
	if cfg.OutboxPublisherEnabled {
		brokers := splitBrokers(cfg.KafkaBrokers)
		sender = messaging.NewSender(brokers, cfg.KafkaTopic)
		publisher = appoutbox.New(appoutbox.Deps{
			UOW: uow, Outbox: postgres.NewOutboxRepo(), Sender: sender, Clock: clk,
			Owner: "system-core", Batch: cfg.OutboxBatch,
			ClaimLease: cfg.OutboxClaimLease, PollInterval: cfg.OutboxPollInterval,
		})
		auditUC := appoutbox.NewConsumer(appoutbox.ConsumerDeps{
			UOW: uow, Idem: postgres.NewIdempotencyRepo(), Clock: clk,
		})
		consumer = messaging.NewConsumer(brokers, cfg.KafkaTopic, "system.audit.v1",
			func(ctx context.Context, body []byte) error {
				ev, err := messaging.Decode(body)
				if err != nil {
					return err
				}
				return auditUC.Handle(ctx, ev)
			})
	}

	srv := &http.Server{
		Addr:         cfg.HTTPAddr,
		Handler:      engine,
		ReadTimeout:  cfg.HTTPReadTimeout,
		WriteTimeout: cfg.HTTPWriteTimeout,
		IdleTimeout:  cfg.HTTPIdleTimeout,
	}
	return &App{server: srv, cfg: cfg, pool: pool, sender: sender, publisher: publisher, consumer: consumer, probeEngine: probeEngine}, nil
}

// episodeDriverAdapter 把 episode 用例转调为 monitor.EpisodeDriver。
type episodeDriverAdapter struct{ uc *appepisode.Service }

func (a episodeDriverAdapter) OpenOrReuse(ctx context.Context, serviceID string) (bool, error) {
	_, created, err := a.uc.OpenOrReuse(ctx, serviceID)
	return created, err
}

func (a episodeDriverAdapter) TryResolve(ctx context.Context, episodeID string) (bool, []string, error) {
	return a.uc.TryResolve(ctx, episodeID)
}

func splitBrokers(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
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
