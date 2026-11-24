package bootstrap

import (
	"net/http"

	"github.com/ecakeman/alert/services/system-core/internal/application/health"
	"github.com/ecakeman/alert/services/system-core/internal/infrastructure/persistence/postgres"
	"github.com/ecakeman/alert/services/system-core/internal/transport/http/handler"
	"github.com/ecakeman/alert/services/system-core/internal/transport/http/router"
)

// API 承载 HTTP + gRPC 角色装配：请求驱动，不启动后台循环。
type API struct {
	Server     *http.Server
	GRPCServer *GRPCServer
}

func NewAPI(sh *Shared, r *Repos, ucs *UseCases) (*API, error) {
	if err := sh.Cfg.ValidateAPIRuntime(); err != nil {
		return nil, err
	}
	healthH := handler.NewHealth(health.NewService(
		postgres.NewPGChecker(sh.Pool, sh.Cfg.ReadyzTimeout),
		postgres.NewMigrationChecker(sh.Pool, sh.Cfg.ReadyzTimeout),
	))
	engine := router.New(router.Deps{
		Health: healthH, Service: handler.NewService(ucs.Service),
		Target:    handler.NewTarget(ucs.Service),
		Dashboard: handler.NewDashboard(ucs.Dashboard),
		Episode:   handler.NewEpisode(ucs.Episode),
		Artifact:  handler.NewArtifact(ucs.Task),
		Retry:     handler.NewRetry(ucs.Task),
		Timeout:   sh.Cfg.HTTPReadTimeout,
	})
	server := &http.Server{
		Addr:         sh.Cfg.HTTPAddr,
		Handler:      engine,
		ReadTimeout:  sh.Cfg.HTTPReadTimeout,
		WriteTimeout: sh.Cfg.HTTPWriteTimeout,
		IdleTimeout:  sh.Cfg.HTTPIdleTimeout,
	}
	grpcServer, err := NewGRPCServer(sh, ucs)
	if err != nil {
		return nil, err
	}
	return &API{Server: server, GRPCServer: grpcServer}, nil
}
