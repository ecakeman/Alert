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

	"github.com/ecakeman/alert/services/platform/db"
	"github.com/ecakeman/alert/services/platform/httpx"
	"github.com/ecakeman/alert/services/platform/logging"
	platformmigrate "github.com/ecakeman/alert/services/platform/migrate"

	"github.com/ecakeman/alert/services/monitor-svc/internal/config"
	monitorpostgres "github.com/ecakeman/alert/services/monitor-svc/internal/infrastructure/persistence/postgres"
	monitorhttp "github.com/ecakeman/alert/services/monitor-svc/internal/transport/http"
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

	health := httpx.NewHealthService(cfg.ServiceName, map[string]httpx.Checker{
		"database":  pool,
		"migration": monitorpostgres.MigrationChecker{Pool: pool.Raw(), Timeout: cfg.ReadyzTimeout},
	})
	health.CheckTimeout = cfg.ReadyzTimeout

	handler := httpx.Chain(
		monitorhttp.New(monitorhttp.Deps{Health: health, Logger: logger}),
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