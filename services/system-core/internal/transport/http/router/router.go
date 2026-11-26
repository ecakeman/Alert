package router

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ecakeman/alert/services/system-core/internal/application/apperr"
	"github.com/ecakeman/alert/services/system-core/internal/transport/http/handler"
	"github.com/ecakeman/alert/services/system-core/internal/transport/http/middleware"
	"github.com/ecakeman/alert/services/system-core/internal/transport/http/response"
)

// Deps 是路由装配依赖。
type Deps struct {
	Health    *handler.Health
	Service   *handler.Service
	Target    *handler.Target
	Dashboard *handler.Dashboard
	Episode   *handler.Episode
	Artifact  *handler.Artifact
	Retry     *handler.Retry
	Timeout   time.Duration
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
	v1.GET("/services/:service_id/targets", deps.Target.ListTargets)
	v1.POST("/services/:service_id/targets", deps.Target.CreateTarget)
	v1.PATCH("/targets/:target_id", deps.Target.PatchTarget)
	v1.DELETE("/targets/:target_id", deps.Target.DeleteTarget)
	v1.GET("/episodes", deps.Episode.List)
	v1.GET("/episodes/:episode_id", deps.Episode.Get)
	v1.POST("/services/:service_id/events", deps.Episode.AppendEvent)
	v1.GET("/services/:service_id/timeline", deps.Episode.ServiceTimeline)
	v1.GET("/episodes/:episode_id/timeline", deps.Episode.Timeline)
	v1.GET("/episodes/:episode_id/artifacts", deps.Artifact.Get)
	v1.POST("/episodes/:episode_id/retry-diagnosis", deps.Retry.RetryDiagnosis)
	v1.POST("/episodes/:episode_id/retry-archive", deps.Retry.RetryArchive)

	// P4 过渡：供 episode-svc 事务外取数的只读域间端点；P5 task 域接管后删除。
	internal := r.Group("/internal")
	internal.GET("/episodes/:episode_id/active-task", deps.Episode.ActiveTask)

	r.NoRoute(func(c *gin.Context) {
		response.Error(c, http.StatusNotFound, apperr.CodeNotFound, "resource not found")
	})
	return r
}
