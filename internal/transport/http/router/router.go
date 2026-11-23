package router

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ecakeman/alert/internal/application/apperr"
	"github.com/ecakeman/alert/internal/transport/http/handler"
	"github.com/ecakeman/alert/internal/transport/http/middleware"
	"github.com/ecakeman/alert/internal/transport/http/response"
)

// Deps 是路由装配依赖。
type Deps struct {
	Health  *handler.Health
	Timeout time.Duration
}

type route struct {
	method  string
	path    string
	handler gin.HandlerFunc
}

// apiV1Routes 是 /api/v1 下的 18 条业务路由；B4/B7 逐条替换 handler。
var apiV1Routes = []route{
	{"GET", "/dashboard/summary", handler.NotImplemented},
	{"GET", "/services", handler.NotImplemented},
	{"POST", "/services", handler.NotImplemented},
	{"GET", "/services/:service_id", handler.NotImplemented},
	{"PATCH", "/services/:service_id", handler.NotImplemented},
	{"DELETE", "/services/:service_id", handler.NotImplemented},
	{"GET", "/services/:service_id/targets", handler.NotImplemented},
	{"POST", "/services/:service_id/targets", handler.NotImplemented},
	{"PATCH", "/targets/:target_id", handler.NotImplemented},
	{"DELETE", "/targets/:target_id", handler.NotImplemented},
	{"GET", "/episodes", handler.NotImplemented},
	{"GET", "/services/:service_id/timeline", handler.NotImplemented},
	{"POST", "/services/:service_id/events", handler.NotImplemented},
	{"GET", "/episodes/:episode_id", handler.NotImplemented},
	{"GET", "/episodes/:episode_id/timeline", handler.NotImplemented},
	{"GET", "/episodes/:episode_id/artifacts", handler.NotImplemented},
	{"POST", "/episodes/:episode_id/retry-diagnosis", handler.NotImplemented},
	{"POST", "/episodes/:episode_id/retry-archive", handler.NotImplemented},
}

// New 是唯一公开路由注册处：中间件链 → 探针 → /api/v1 壳表 → 统一 404。
func New(deps Deps) *gin.Engine {
	r := gin.New()
	r.Use(
		middleware.Recovery(),
		middleware.RequestID(),
		middleware.AccessLog(),
		middleware.Timeout(deps.Timeout),
	)

	r.GET("/healthz", deps.Health.Live)
	r.GET("/readyz", deps.Health.Ready)

	v1 := r.Group("/api/v1")
	for _, rt := range apiV1Routes {
		v1.Handle(rt.method, rt.path, rt.handler)
	}

	r.NoRoute(func(c *gin.Context) {
		response.Error(c, http.StatusNotFound, apperr.CodeNotFound, "resource not found")
	})
	return r
}