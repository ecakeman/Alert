package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/ecakeman/alert/services/platform/db"
	"github.com/ecakeman/alert/services/platform/httpx"
	"github.com/ecakeman/alert/services/platform/logging"

	"github.com/ecakeman/alert/services/service-registry/internal/config"
	registrypostgres "github.com/ecakeman/alert/services/service-registry/internal/infrastructure/persistence/postgres"
	registryhttp "github.com/ecakeman/alert/services/service-registry/internal/transport/http"
)

func main() {
	healthcheck := flag.Bool("healthcheck", false, "probe local /readyz and exit")
	flag.Parse()

	cfg, err := config.Load(os.Getenv)
	if err != nil {
		log.Printf("config: %v", err)
		os.Exit(1)
	}

	if *healthcheck {
		os.Exit(httpx.Run(healthcheckAddr(cfg.HTTPAddr)))
	}

	logger, err := logging.New(logging.Config{Service: cfg.ServiceName, Level: cfg.LogLevel, JSON: true})
	if err != nil {
		log.Printf("logging: %v", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.New(ctx, cfg.DBConfig())
	if err != nil {
		logger.Error("database config", "reason", err.Error())
		os.Exit(1)
	}
	defer pool.Close()

	health := httpx.NewHealthService(cfg.ServiceName, map[string]httpx.Checker{
		"database":  pool,
		"migration": registrypostgres.MigrationChecker{Pool: pool.Raw(), Timeout: cfg.ReadyzTimeout},
	})
	health.CheckTimeout = cfg.ReadyzTimeout

	handler := httpx.Chain(
		registryhttp.New(registryhttp.Deps{Health: health, Logger: logger}),
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
