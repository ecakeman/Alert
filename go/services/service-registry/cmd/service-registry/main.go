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

	"github.com/gin-gonic/gin"

	registryconsumer "github.com/ecakeman/alert/services/service-registry/internal/application/consumer"
	appservice "github.com/ecakeman/alert/services/service-registry/internal/application/service"
	"github.com/ecakeman/alert/services/service-registry/internal/config"
	"github.com/ecakeman/alert/services/service-registry/internal/infrastructure/clock"
	"github.com/ecakeman/alert/services/service-registry/internal/infrastructure/idgen"
	"github.com/ecakeman/alert/services/service-registry/internal/infrastructure/outboxsender"
	registrypostgres "github.com/ecakeman/alert/services/service-registry/internal/infrastructure/persistence/postgres"
	registryhttp "github.com/ecakeman/alert/services/service-registry/internal/transport/http"
	registryhandler "github.com/ecakeman/alert/services/service-registry/internal/transport/http/handler"
	"github.com/ecakeman/alert/services/service-registry/migrations"
)

func main() {
	healthcheck := flag.Bool("healthcheck", false, "probe local /readyz and exit")
	migrateOnly := flag.Bool("migrate", false, "run registry migrations then exit")
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
			Schema:       "registry",
			VersionTable: "registry.schema_migrations",
			LockKey:      "alert.registry.migrate",
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

	health := httpx.NewHealthService(cfg.ServiceName, map[string]httpx.Checker{
		"database":  pool,
		"migration": registrypostgres.MigrationChecker{Pool: pool.Raw(), Timeout: cfg.ReadyzTimeout},
	})
	health.CheckTimeout = cfg.ReadyzTimeout

	uc := appservice.New(appservice.Deps{
		UOW:      registrypostgres.NewUnitOfWork(pool.Raw()),
		Services: registrypostgres.NewServiceRepo(),
		Targets:  registrypostgres.NewTargetRepo(),
		Sources:  registrypostgres.NewSourceStateRepo(),
		Outbox:   registrypostgres.NewOutboxRepo(),
		Clock:    clock.System{},
		IDs:      idgen.Generator{},
	})
	engine := gin.New()
	engine.Use(gin.Recovery())
	registryhandler.NewRegistry(uc).Register(engine)

	if cfg.KafkaBrokers != "" {
		sender, err := messaging.NewSender(messaging.SenderConfig{
			Brokers: splitBrokers(cfg.KafkaBrokers), Topic: cfg.KafkaPublishTopic,
		})
		if err != nil {
			logger.Error("kafka sender", "reason", err.Error())
			os.Exit(1)
		}
		outboxRepo, err := platformoutbox.NewRepo("registry")
		if err != nil {
			logger.Error("outbox repo", "reason", err.Error())
			os.Exit(1)
		}
		publisher, err := platformoutbox.New(platformoutbox.Deps{
			Store: outboxRepo, Tx: pool.Raw(),
			Sender: outboxsender.New(sender), Owner: "registry",
			PublishTopic: cfg.KafkaPublishTopic,
			ClaimLease:   30 * time.Second, Batch: 16, PollInterval: 500 * time.Millisecond,
		})
		if err != nil {
			logger.Error("outbox publisher", "reason", err.Error())
			os.Exit(1)
		}
		go publisher.Run(ctx)
		defer sender.Close()

		registryConsumer := registryconsumer.New(registryconsumer.Deps{
			UOW:     registrypostgres.NewUnitOfWork(pool.Raw()),
			Targets: registrypostgres.NewTargetRepo(),
			Sources: registrypostgres.NewSourceStateRepo(),
			Idem:    registrypostgres.NewIdempotencyRepo(),
			Clock:   clock.System{},
		})
		kafkaConsumer, err := messaging.NewConsumer(messaging.ConsumerConfig{
			Brokers: splitBrokers(cfg.KafkaBrokers), Topics: cfg.KafkaConsumeTopics,
			GroupID: "registry.monitor-events.v1", Logger: logger,
		}, registryConsumer.Handle)
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

	handler := httpx.Chain(
		registryhttp.New(registryhttp.Deps{Health: health, Logger: logger, API: engine}),
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
	logger.Info("service-registry listening", "addr", cfg.HTTPAddr)

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

func healthcheckAddr(addr string) string {
	addr = strings.TrimSpace(addr)
	if strings.HasPrefix(addr, ":") {
		return "127.0.0.1" + addr
	}
	if !strings.Contains(addr, ":") {
		return addr + ":8081"
	}
	return addr
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
