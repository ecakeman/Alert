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
	Health    *handler.Health
	Service   *handler.Service
	Dashboard *handler.Dashboard
	Episode   *handler.Episode
	Timeout   time.Duration
}

type route struct {
	method string
	path   string
}

// pendingRoutes 是尚未施工的业务路由（B4.2/B4.3/B4.4 及 B7），保持 501。
var pendingRoutes = []route{
	{"GET", "/episodes"},
	{"GET", "/services/:service_id/timeline"},
	{"GET", "/episodes/:episode_id/timeline"},
	{"GET", "/episodes/:episode_id/artifacts"},
	{"POST", "/episodes/:episode_id/retry-diagnosis"},
	{"POST", "/episodes/:episode_id/retry-archive"},
}

// New 是唯一公开路由注册处：中间件链 → 探针 → 已实现路由 → 未施工 501 → 统一 404。
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
	v1.GET("/dashboard/summary", deps.Dashboard.Summary)
	v1.GET("/services", deps.Service.List)
	v1.POST("/services", deps.Service.Create)
	v1.GET("/services/:service_id", deps.Service.Get)
	v1.PATCH("/services/:service_id", deps.Service.Patch)
	v1.DELETE("/services/:service_id", deps.Service.Delete)
	v1.GET("/services/:service_id/targets", deps.Service.ListTargets)
	v1.POST("/services/:service_id/targets", deps.Service.CreateTarget)
	v1.PATCH("/targets/:target_id", deps.Service.PatchTarget)
	v1.DELETE("/targets/:target_id", deps.Service.DeleteTarget)
	v1.GET("/episodes/:episode_id", deps.Episode.Get)
	v1.POST("/services/:service_id/events", deps.Episode.AppendEvent)

	for _, rt := range pendingRoutes {
		v1.Handle(rt.method, rt.path, handler.NotImplemented)
	}

	r.NoRoute(func(c *gin.Context) {
		response.Error(c, http.StatusNotFound, apperr.CodeNotFound, "resource not found")
	})
	return r
}
