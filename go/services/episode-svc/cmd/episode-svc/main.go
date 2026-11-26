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

	"github.com/ecakeman/alert/services/episode-svc/internal/config"
	episodeconsumer "github.com/ecakeman/alert/services/episode-svc/internal/application/consumer"
	episodeapp "github.com/ecakeman/alert/services/episode-svc/internal/application/episode"
	"github.com/ecakeman/alert/services/episode-svc/internal/infrastructure/clock"
	"github.com/ecakeman/alert/services/episode-svc/internal/infrastructure/idgen"
	"github.com/ecakeman/alert/services/episode-svc/internal/infrastructure/monitorclient"
	"github.com/ecakeman/alert/services/episode-svc/internal/infrastructure/outboxsender"
	"github.com/ecakeman/alert/services/episode-svc/internal/infrastructure/registryclient"
	"github.com/ecakeman/alert/services/episode-svc/internal/infrastructure/taskclient"
	episodepostgres "github.com/ecakeman/alert/services/episode-svc/internal/infrastructure/persistence/postgres"
	episodehttp "github.com/ecakeman/alert/services/episode-svc/internal/transport/http"
	episodehandler "github.com/ecakeman/alert/services/episode-svc/internal/transport/http/handler"
	"github.com/ecakeman/alert/services/episode-svc/migrations"
	"github.com/ecakeman/alert/services/platform/db"
	"github.com/ecakeman/alert/services/platform/httpx"
	"github.com/ecakeman/alert/services/platform/messaging"
	platformmigrate "github.com/ecakeman/alert/services/platform/migrate"
	platformoutbox "github.com/ecakeman/alert/services/platform/outbox"
	"github.com/gin-gonic/gin"
)

func main() {
	healthcheck := flag.Bool("healthcheck", false, "probe local /readyz and exit")
	migrateOnly := flag.Bool("migrate", false, "run episode migrations then exit")
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
			Schema:       "episode",
			VersionTable: "episode.schema_migrations",
			LockKey:      "alert.episode.migrate",
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

	health := httpx.NewHealthService(cfg.ServiceName, map[string]httpx.Checker{
		"database":  pool,
		"migration": episodepostgres.MigrationChecker{Pool: pool.Raw(), Timeout: cfg.ReadyzTimeout},
	})
	health.CheckTimeout = cfg.ReadyzTimeout

	uow := episodepostgres.NewUnitOfWork(pool.Raw())
	svc := episodeapp.New(episodeapp.Deps{
		UOW:           uow,
		Episodes:      episodepostgres.NewEpisodeRepo(),
		InputVersions: episodepostgres.NewInputVersionRepo(),
		AppEvents:     episodepostgres.NewAppEventRepo(),
		Outbox:        episodepostgres.NewOutboxRepo(),
		Registry:      registryclient.New(cfg.RegistryBaseURL, cfg.UpstreamTimeout),
		Monitor:       monitorclient.New(cfg.MonitorBaseURL, cfg.UpstreamTimeout),
		Tasks:         taskclient.New(cfg.TaskBaseURL, cfg.UpstreamTimeout),
		Clock:         clock.System{},
		IDs:           idgen.Generator{},
	})

	if cfg.KafkaBrokers != "" {
		brokers := splitBrokers(cfg.KafkaBrokers)
		sender, err := messaging.NewSender(messaging.SenderConfig{Brokers: brokers, Topic: cfg.KafkaPublishTopic})
		if err != nil {
			log.Printf("kafka sender: %v", err)
			os.Exit(1)
		}
		outboxRepo, err := platformoutbox.NewRepo("episode")
		if err != nil {
			log.Printf("outbox repo: %v", err)
			os.Exit(1)
		}
		publisher, err := platformoutbox.New(platformoutbox.Deps{
			Store: outboxRepo, Tx: pool.Raw(), Sender: outboxsender.New(sender), Owner: "episode",
			PublishTopic: cfg.KafkaPublishTopic, ClaimLease: 30 * time.Second, Batch: 16,
			PollInterval: 500 * time.Millisecond,
		})
		if err != nil {
			log.Printf("outbox publisher: %v", err)
			os.Exit(1)
		}
		go publisher.Run(ctx)
		defer sender.Close()

		epConsumer := episodeconsumer.New(episodeconsumer.Deps{
			UOW: uow, Idem: episodepostgres.NewIdempotencyRepo(), Episodes: svc, Clock: clock.System{},
		})
		kafkaConsumer, err := messaging.NewConsumer(messaging.ConsumerConfig{
			Brokers: brokers, Topics: cfg.KafkaConsumeTopics,
			GroupID: "episode.monitor-events.v1",
		}, epConsumer.Handle)
		if err != nil {
			log.Printf("kafka consumer: %v", err)
			os.Exit(1)
		}
		go func() {
			if err := kafkaConsumer.Run(ctx); err != nil && ctx.Err() == nil {
				log.Printf("kafka consumer stopped: %v", err)
			}
		}()
		defer kafkaConsumer.Close()
	}

	engine := gin.New()
	engine.Use(gin.Recovery())
	episodehandler.NewEpisode(svc, clock.System{}).Register(engine)

	handler := httpx.Chain(
		episodehttp.New(episodehttp.Deps{Health: health, API: engine}),
		httpx.Recovery(nil),
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
	log.Printf("episode-svc listening addr=%s", cfg.HTTPAddr)

	select {
	case err := <-errCh:
		log.Printf("server stopped: %v", err)
		os.Exit(1)
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.HTTPShutdownTimeout)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}
}

func healthcheckAddr(addr string) string {
	addr = strings.TrimSpace(addr)
	if strings.HasPrefix(addr, ":") {
		return "127.0.0.1" + addr
	}
	if !strings.Contains(addr, ":") {
		return addr + ":8082"
	}
	return addr
}

func splitBrokers(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}
