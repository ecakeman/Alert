package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	googlegrpc "google.golang.org/grpc"

	"github.com/ecakeman/alert/services/platform/db"
	"github.com/ecakeman/alert/services/platform/httpx"
	"github.com/ecakeman/alert/services/platform/messaging"
	platformmigrate "github.com/ecakeman/alert/services/platform/migrate"
	platformoutbox "github.com/ecakeman/alert/services/platform/outbox"

	"github.com/ecakeman/alert/services/task-orchestrator/internal/config"
	taskconsumer "github.com/ecakeman/alert/services/task-orchestrator/internal/application/consumer"
	taskapp "github.com/ecakeman/alert/services/task-orchestrator/internal/application/task"
	"github.com/ecakeman/alert/services/task-orchestrator/internal/infrastructure/artifacts"
	"github.com/ecakeman/alert/services/task-orchestrator/internal/infrastructure/clock"
	"github.com/ecakeman/alert/services/task-orchestrator/internal/infrastructure/episodeclient"
	"github.com/ecakeman/alert/services/task-orchestrator/internal/infrastructure/grpcclient"
	"github.com/ecakeman/alert/services/task-orchestrator/internal/infrastructure/idgen"
	"github.com/ecakeman/alert/services/task-orchestrator/internal/infrastructure/outboxsender"
	taskpostgres "github.com/ecakeman/alert/services/task-orchestrator/internal/infrastructure/persistence/postgres"
	taskhttp "github.com/ecakeman/alert/services/task-orchestrator/internal/transport/http"
	taskhandler "github.com/ecakeman/alert/services/task-orchestrator/internal/transport/http/handler"
	transportgrpc "github.com/ecakeman/alert/services/task-orchestrator/internal/transport/grpc"
	"github.com/ecakeman/alert/services/task-orchestrator/migrations"
)

func main() {
	healthcheck := flag.Bool("healthcheck", false, "probe local /readyz and exit")
	migrateOnly := flag.Bool("migrate", false, "run task migrations then exit")
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
			Schema:       "task",
			VersionTable: "task.schema_migrations",
			LockKey:      "alert.task.migrate",
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
		"migration": taskpostgres.MigrationChecker{Pool: pool.Raw(), Timeout: cfg.ReadyzTimeout},
	})
	health.CheckTimeout = cfg.ReadyzTimeout

	epClient := episodeclient.New(cfg.EpisodeBaseURL, cfg.UpstreamTimeout)
	uow := taskpostgres.NewUnitOfWork(pool.Raw())
	svc := taskapp.New(taskapp.Deps{
		UOW: uow, Tasks: taskpostgres.NewTaskRepo(), Ownership: taskpostgres.NewOwnershipRepo(),
		Artifacts: taskpostgres.NewArtifactRepo(), Outbox: taskpostgres.NewOutboxRepo(),
		Idem: taskpostgres.NewIdempotencyRepo(), Clock: clock.System{}, IDs: idgen.Generator{},
		Lease: 30 * time.Second, MaxAttempts: 3, RetryDelay: 5 * time.Second, Deadline: 5 * time.Minute,
		Dispatch:   epClient,
		EpisodeGet: epClient.GetEpisode,
		Submit:     grpcclient.NewAgentClient(cfg.AgentGRPCTarget, cfg.UpstreamTimeout),
		Reader:     artifacts.NewReader(cfg.ArtifactStorageRoot),
	})

	var kafkaConsumer *messaging.Consumer
	var kafkaSender *messaging.Sender
	if cfg.KafkaBrokers != "" {
		brokers := splitBrokers(cfg.KafkaBrokers)
		sender, err := messaging.NewSender(messaging.SenderConfig{Brokers: brokers, Topic: cfg.KafkaPublishTopic})
		if err != nil {
			log.Printf("kafka sender: %v", err)
			os.Exit(1)
		}
		kafkaSender = sender
		outboxRepo, err := platformoutbox.NewRepo("task")
		if err != nil {
			log.Printf("outbox repo: %v", err)
			os.Exit(1)
		}
		publisher, err := platformoutbox.New(platformoutbox.Deps{
			Store: outboxRepo, Tx: pool.Raw(), Sender: outboxsender.New(sender), Owner: "task",
			PublishTopic: cfg.KafkaPublishTopic, ClaimLease: 30 * time.Second, Batch: 16,
			PollInterval: 500 * time.Millisecond,
		})
		if err != nil {
			log.Printf("outbox publisher: %v", err)
			os.Exit(1)
		}
		go publisher.Run(ctx)

		kc, err := messaging.NewConsumer(messaging.ConsumerConfig{
			Brokers: brokers, Topics: cfg.KafkaConsumeTopics, GroupID: "task.dispatch.v1",
		}, taskconsumer.New(svc).Handle)
		if err != nil {
			log.Printf("kafka consumer: %v", err)
			os.Exit(1)
		}
		kafkaConsumer = kc
		go func() {
			if err := kc.Run(ctx); err != nil && ctx.Err() == nil {
				log.Printf("kafka consumer stopped: %v", err)
			}
		}()
	}
	defer func() {
		if kafkaConsumer != nil {
			kafkaConsumer.Close()
		}
		if kafkaSender != nil {
			kafkaSender.Close()
		}
	}()

	lis, err := net.Listen("tcp", cfg.GRPCAddr)
	if err != nil {
		log.Printf("grpc listen: %v", err)
		os.Exit(1)
	}
	grpcServer := googlegrpc.NewServer(googlegrpc.UnaryInterceptor(transportgrpc.UnaryInterceptor()))
	transportgrpc.Register(grpcServer, transportgrpc.NewServer(svc))
	go func() {
		if err := grpcServer.Serve(lis); err != nil && ctx.Err() == nil {
			log.Printf("grpc serve: %v", err)
		}
	}()

	go func() {
		ticker := time.NewTicker(cfg.ReconcileInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				summary, err := svc.Reconcile(ctx, time.Now().UTC())
				if err != nil {
					log.Printf("reconcile: %v", err)
					continue
				}
				if len(summary.Failures) > 0 {
					log.Printf("reconcile failures=%v", summary.Failures)
				}
			}
		}
	}()

	engine := gin.New()
	engine.Use(gin.Recovery())
	taskhandler.NewInternal(svc).Register(engine)

	handler := httpx.Chain(
		taskhttp.New(taskhttp.Deps{Health: health, API: engine}),
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
	log.Printf("task-orchestrator listening http=%s grpc=%s", cfg.HTTPAddr, cfg.GRPCAddr)

	select {
	case err := <-errCh:
		log.Printf("server stopped: %v", err)
		os.Exit(1)
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.HTTPShutdownTimeout)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		grpcServer.GracefulStop()
	}
}

func healthcheckAddr(addr string) string {
	addr = strings.TrimSpace(addr)
	if strings.HasPrefix(addr, ":") {
		return "http://127.0.0.1" + addr
	}
	if strings.HasPrefix(addr, "0.0.0.0:") {
		return "http://127.0.0.1:" + strings.TrimPrefix(addr, "0.0.0.0:")
	}
	return "http://" + addr
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
