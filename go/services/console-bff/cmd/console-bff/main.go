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
	"time"

	"github.com/ecakeman/alert/services/platform/httpx"
	"github.com/ecakeman/alert/services/platform/logging"

	"github.com/ecakeman/alert/services/console-bff/internal/aggregate"
	"github.com/ecakeman/alert/services/console-bff/internal/config"
	bffhttp "github.com/ecakeman/alert/services/console-bff/internal/transport/http"
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

	logger, err := logging.New(logging.Config{
		Service: cfg.ServiceName,
		Level:   cfg.LogLevel,
		JSON:    true,
	})
	if err != nil {
		log.Printf("logging: %v", err)
		os.Exit(1)
	}

	aggregateSvc := aggregate.New(aggregate.Clients{
		Registry: httpx.NewClient(cfg.RegistryBaseURL, cfg.UpstreamTimeout),
		Episode:  httpx.NewClient(cfg.EpisodeBaseURL, cfg.UpstreamTimeout),
		Monitor:  httpx.NewClient(cfg.MonitorBaseURL, cfg.UpstreamTimeout),
		Task:     httpx.NewClient(cfg.TaskBaseURL, cfg.UpstreamTimeout),
	}, cfg.AggregateTimeout, logger)

	proxy, err := bffhttp.NewProxy(bffhttp.ProxyConfig{
		Registry:  cfg.RegistryBaseURL,
		Monitor:   cfg.MonitorBaseURL,
		Episode:   cfg.EpisodeBaseURL,
		Task:      cfg.TaskBaseURL,
		Aggregate: aggregateSvc,
		Timeout:   cfg.UpstreamTimeout,
		Logger:    logger,
	})
	if err != nil {
		logger.Error("proxy", "reason", err.Error())
		os.Exit(1)
	}

	health := httpx.NewHealthService(cfg.ServiceName, map[string]httpx.Checker{
		"registry": downstream("registry", cfg.RegistryBaseURL, cfg.UpstreamTimeout),
		"monitor":  downstream("monitor", cfg.MonitorBaseURL, cfg.UpstreamTimeout),
		"episode":  downstream("episode", cfg.EpisodeBaseURL, cfg.UpstreamTimeout),
		"task":     downstream("task", cfg.TaskBaseURL, cfg.UpstreamTimeout),
	})
	health.CheckTimeout = cfg.ReadyzTimeout

	handler := httpx.Chain(
		bffhttp.New(bffhttp.Deps{Health: health, Logger: logger, Proxy: proxy, Timeout: cfg.HTTPReadTimeout}),
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	logger.Info("console-bff listening", "addr", cfg.HTTPAddr)
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

func downstream(name, baseURL string, timeout time.Duration) *bffhttp.DownstreamChecker {
	return &bffhttp.DownstreamChecker{
		Name:     name,
		Client:   httpx.NewClient(baseURL, timeout),
		CacheTTL: 10 * time.Second,
	}
}

func healthcheckAddr(addr string) string {
	addr = strings.TrimSpace(addr)
	if strings.HasPrefix(addr, ":") {
		return "127.0.0.1" + addr
	}
	if !strings.Contains(addr, ":") {
		return addr + ":8080"
	}
	return addr
}
