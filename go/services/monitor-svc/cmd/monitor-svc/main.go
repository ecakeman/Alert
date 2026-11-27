package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/ecakeman/alert/services/platform/db"
	"github.com/ecakeman/alert/services/platform/httpx"
	"github.com/ecakeman/alert/services/platform/logging"
	"github.com/ecakeman/alert/services/platform/messaging"
	platformmigrate "github.com/ecakeman/alert/services/platform/migrate"
	platformoutbox "github.com/ecakeman/alert/services/platform/outbox"
	"github.com/jackc/pgx/v5/pgxpool"

	monitorapp "github.com/ecakeman/alert/services/monitor-svc/internal/application/monitor"
	portmonitor "github.com/ecakeman/alert/services/monitor-svc/internal/application/port/monitor"
	monitorprojection "github.com/ecakeman/alert/services/monitor-svc/internal/application/projection"
	"github.com/ecakeman/alert/services/monitor-svc/internal/config"
	"github.com/ecakeman/alert/services/monitor-svc/internal/infrastructure/clock"
	"github.com/ecakeman/alert/services/monitor-svc/internal/infrastructure/idgen"
	"github.com/ecakeman/alert/services/monitor-svc/internal/infrastructure/outboxsender"
	monitorpostgres "github.com/ecakeman/alert/services/monitor-svc/internal/infrastructure/persistence/postgres"
	"github.com/ecakeman/alert/services/monitor-svc/internal/infrastructure/probe"
	monitorhttp "github.com/ecakeman/alert/services/monitor-svc/internal/transport/http"
	monitorhandler "github.com/ecakeman/alert/services/monitor-svc/internal/transport/http/handler"

	"github.com/gin-gonic/gin"

	"github.com/ecakeman/alert/services/monitor-svc/migrations"
)

func main() {
	healthcheck := flag.Bool("healthcheck", false, "probe local /readyz and exit")
	migrateOnly := flag.Bool("migrate", false, "run monitor migrations then exit")
	flag.Parse()

	cfg, err := config.Load(os.Getenv)
	if err != nil {
		log.Printf("config: %v", err)
		os.Exit(1)
	}

	if *healthcheck {
		os.Exit(httpx.Run(healthcheckAddr(cfg.HTTPAddr)))
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.New(ctx, cfg.DBConfig())
	if err != nil {
		log.Printf("database config: %v", err)
		os.Exit(1)
	}
	defer pool.Close()

	if *migrateOnly {
		waitCtx, cancel := context.WithTimeout(ctx, cfg.DBConnectTimeout)
		waitErr := pool.WaitReady(waitCtx)
		cancel()
		if waitErr != nil {
			log.Printf("migrate: database not ready: %v", waitErr)
			os.Exit(1)
		}
		result, err := platformmigrate.Up(ctx, pool.Raw(), platformmigrate.Target{
			Schema:       "monitor",
			VersionTable: "monitor.schema_migrations",
			LockKey:      "alert.monitor.migrate",
			FS:           migrations.FS,
		})
		if err != nil {
			log.Printf("migrate: %v", err)
			os.Exit(1)
		}
		if len(result.Conflicts) > 0 {
			log.Printf("migration conflicts: %v", result.Conflicts)
			os.Exit(1)
		}
		fmt.Printf("applied=%d skipped=%d conflicts=%d\n", len(result.Applied), len(result.Skipped), len(result.Conflicts))
		return
	}

	logger, err := logging.New(logging.Config{Service: cfg.ServiceName, Level: cfg.LogLevel, JSON: true})
	if err != nil {
		log.Printf("logging: %v", err)
		os.Exit(1)
	}

	synced := monitorprojection.NewSyncState()
	// 重启后以持久化消费记录判定"已同步"，避免 readyz 永久 503。
	if catalogSyncedBefore(ctx, pool.Raw()) {
		synced.Mark()
	}
	health := httpx.NewHealthService(cfg.ServiceName, map[string]httpx.Checker{
		"database":  pool,
		"migration": monitorpostgres.MigrationChecker{Pool: pool.Raw(), Timeout: cfg.ReadyzTimeout},
		"catalog":   synced,
	})
	health.CheckTimeout = cfg.ReadyzTimeout

	if cfg.KafkaBrokers != "" {
		sender, err := messaging.NewSender(messaging.SenderConfig{
			Brokers: splitBrokers(cfg.KafkaBrokers), Topic: cfg.KafkaPublishTopic,
		})
		if err != nil {
			logger.Error("kafka sender", "reason", err.Error())
			os.Exit(1)
		}
		outboxRepo, err := platformoutbox.NewRepo("monitor")
		if err != nil {
			logger.Error("outbox repo", "reason", err.Error())
			os.Exit(1)
		}
		publisher, err := platformoutbox.New(platformoutbox.Deps{
			Store: outboxRepo, Tx: pool.Raw(),
			Sender: outboxsender.New(sender), Owner: "monitor",
			PublishTopic: cfg.KafkaPublishTopic,
			ClaimLease:   30 * time.Second, Batch: 16, PollInterval: 500 * time.Millisecond,
		})
		if err != nil {
			logger.Error("outbox publisher", "reason", err.Error())
			os.Exit(1)
		}
		go publisher.Run(ctx)
		defer sender.Close()

		catalogConsumer := monitorprojection.New(monitorprojection.Deps{
			UOW:        monitorpostgres.NewUnitOfWork(pool.Raw()),
			Projection: monitorpostgres.NewProjectionRepo(),
			Idem:       monitorpostgres.NewIdempotencyRepo(),
			Clock:      clock.System{},
			Synced:     synced,
		})
		kafkaConsumer, err := messaging.NewConsumer(messaging.ConsumerConfig{
			Brokers: splitBrokers(cfg.KafkaBrokers), Topics: cfg.KafkaConsumeTopics,
			GroupID: "monitor.catalog-events.v1", Logger: logger,
		}, catalogConsumer.Handle)
		if err != nil {
			logger.Error("kafka consumer", "reason", err.Error())
			os.Exit(1)
		}
		go func() {
			if err := kafkaConsumer.Run(ctx); err != nil && ctx.Err() == nil {
				logger.Error("kafka consumer stopped", "reason", err.Error())
			}
		}()
		defer kafkaConsumer.Close()
	}

	uow := monitorpostgres.NewUnitOfWork(pool.Raw())

	if cfg.ProbeEnabled {
		sampler := monitorapp.New(monitorapp.Deps{
			UOW:        uow,
			Points:     monitorpostgres.NewMonitorPointRepo(),
			Projection: monitorpostgres.NewProjectionRepo(),
			States:     monitorpostgres.NewStateRepo(),
			Outbox:     monitorpostgres.NewOutboxRepo(),
			Clock:      clock.System{},
			IDs:        idgen.Generator{},
		})
		engine := probe.NewEngine(probe.EngineDeps{
			Source: sampler, HTTP: probe.NewHTTPProber(), TCP: probe.NewTCPProber(),
			Handler: func(ctx context.Context, snap portmonitor.TargetSnapshot, smp probe.Sample) error {
				_, err := sampler.ApplySample(ctx, snap.ServiceID, snap.TargetID, snap.ConfigGeneration, monitorapp.Sample{
					CheckStatus: smp.CheckStatus, StatusClass: smp.StatusClass, ErrorClass: smp.ErrorClass,
					LatencyMS: smp.LatencyMS, StateFingerprint: smp.StateFingerprint,
				})
				return err
			},
			Workers: cfg.ProbeWorkers, Capacity: cfg.ProbeQueueCapacity,
			Clock: clock.System{}, ScanEvery: cfg.MonitorScanInterval,
		})
		go func() {
			if err := engine.Run(ctx); err != nil && ctx.Err() == nil {
				logger.Error("probe engine stopped", "reason", err.Error())
			}
		}()
		logger.Info("probe engine started", "workers", cfg.ProbeWorkers, "scan_every", cfg.MonitorScanInterval.String())
	}

	engine := gin.New()
	engine.Use(gin.Recovery())
	monitorhandler.NewMonitor(uow, clock.System{}).Register(engine)

	handler := httpx.Chain(
		monitorhttp.New(monitorhttp.Deps{Health: health, Logger: logger, API: engine}),
		httpx.Recovery(logger),
		httpx.Trace(logger),
		httpx.AccessLog(logger),
		httpx.Timeout(cfg.HTTPReadTimeout),
	)

	server := &http.Server{
		Addr:         cfg.HTTPAddr,
		Handler:      handler,
		ReadTimeout:  cfg.HTTPReadTimeout,
		WriteTimeout: cfg.HTTPWriteTimeout,
		IdleTimeout:  cfg.HTTPIdleTimeout,
	}

	errCh := make(chan error, 1)
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()
	logger.Info("monitor-svc listening", "addr", cfg.HTTPAddr)

	select {
	case err := <-errCh:
		logger.Error("server stopped", "reason", err.Error())
		os.Exit(1)
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.HTTPShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("shutdown", "reason", err.Error())
		}
	}
}

func catalogSyncedBefore(ctx context.Context, pool *pgxpool.Pool) bool {
	// 已消费过任一 catalog 事件（幂等表有记录）即视为已同步；
	// 或投影为空（无 target 可同步，空库/空目录是合法已同步态），避免空库冷启动 readyz 永久 503。
	var ok bool
	err := pool.QueryRow(ctx, `
SELECT EXISTS(SELECT 1 FROM monitor.consumer_idempotency WHERE consumer='monitor.catalog-events.v1')
    OR NOT EXISTS(SELECT 1 FROM monitor.monitor_target_projection WHERE deleted_at IS NULL)`).Scan(&ok)
	return err == nil && ok
}

func splitBrokers(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func healthcheckAddr(addr string) string {
	addr = strings.TrimSpace(addr)
	if strings.HasPrefix(addr, ":") {
		return "127.0.0.1" + addr
	}
	if !strings.Contains(addr, ":") {
		return addr + ":8083"
	}
	return addr
}
