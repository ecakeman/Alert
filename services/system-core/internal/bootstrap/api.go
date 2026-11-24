package bootstrap

import (
	"net/http"

	"github.com/ecakeman/alert/services/system-core/internal/application/health"
	"github.com/ecakeman/alert/services/system-core/internal/infrastructure/persistence/postgres"
	"github.com/ecakeman/alert/services/system-core/internal/transport/http/handler"
	"github.com/ecakeman/alert/services/system-core/internal/transport/http/router"
)

// API 承载 HTTP 角色装配：health checker → handler → 路由 → http.Server。
// 角色白名单：仅 HTTP 装配，不装配后台组件（探测/发布/消费链路，出口检查 rg 断言）。
type API struct {
	Server *http.Server
}

func NewAPI(sh *Shared, r *Repos, ucs *UseCases) (*API, error) {
	healthH := handler.NewHealth(health.NewService(
		postgres.NewPGChecker(sh.Pool, sh.Cfg.ReadyzTimeout),
		postgres.NewMigrationChecker(sh.Pool, sh.Cfg.ReadyzTimeout),
	))
	engine := router.New(router.Deps{
		Health: healthH, Service: handler.NewService(ucs.Service),
		Target:    handler.NewTarget(ucs.Service),
		Dashboard: handler.NewDashboard(ucs.Dashboard),
		Episode:   handler.NewEpisode(ucs.Episode),
		Timeout:   sh.Cfg.HTTPReadTimeout,
	})
	srv := &http.Server{
		Addr:         sh.Cfg.HTTPAddr,
		Handler:      engine,
		ReadTimeout:  sh.Cfg.HTTPReadTimeout,
		WriteTimeout: sh.Cfg.HTTPWriteTimeout,
		IdleTimeout:  sh.Cfg.HTTPIdleTimeout,
	}
	return &API{Server: srv}, nil
}
