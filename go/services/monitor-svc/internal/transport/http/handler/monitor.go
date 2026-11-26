package handler

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ecakeman/alert/services/monitor-svc/internal/application/apperr"

	portmonitor "github.com/ecakeman/alert/services/monitor-svc/internal/application/port/monitor"
	portshared "github.com/ecakeman/alert/services/monitor-svc/internal/application/port/shared"
	monitorpostgres "github.com/ecakeman/alert/services/monitor-svc/internal/infrastructure/persistence/postgres"
	"github.com/ecakeman/alert/services/monitor-svc/internal/transport/http/dto"
)

// Monitor 承载 4 个 /internal 域端点。
type Monitor struct {
	uow   portshared.UnitOfWork
	clock portshared.Clock
}

func NewMonitor(uow portshared.UnitOfWork, clock portshared.Clock) *Monitor {
	return &Monitor{uow: uow, clock: clock}
}

// Register 注册 monitor 域 API。
func (h *Monitor) Register(router *gin.Engine) {
	g := router.Group("/internal")
	g.GET("/services/:service_id/points/latest-seq", h.latestSeq)
	g.GET("/services/:service_id/points", h.listPoints)
	g.GET("/points/trend", h.trend)
	g.GET("/stats", h.stats)
}

func (h *Monitor) latestSeq(c *gin.Context) {
	ctx := c.Request.Context()
	repo := monitorpostgres.NewMonitorPointRepo()
	var seq int64
	err := h.uow.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		var err error
		seq, err = repo.LatestSeqByService(ctx, tx, c.Param("service_id"))
		return err
	})
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(200, dto.LatestSeq{LatestSeq: seq})
}

func (h *Monitor) listPoints(c *gin.Context) {
	ctx := c.Request.Context()
	repo := monitorpostgres.NewMonitorPointRepo()
	from := int64Query(c, "from_seq", 0)
	to := int64Query(c, "to_seq", 0)
	if to == 0 {
		to = 1<<63 - 1
	}
	limit := intQuery(c, "limit", 100, 1000)
	var points []portmonitor.MonitorPoint
	err := h.uow.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		var err error
		points, err = repo.ListByBound(ctx, tx, c.Param("service_id"), from, to, limit)
		return err
	})
	if err != nil {
		writeErr(c, err)
		return
	}
	out := make([]dto.MonitorPoint, 0, len(points))
	for _, p := range points {
		out = append(out, dto.MonitorPoint{
			PointSeq: p.PointSeq, TargetID: p.TargetID, TargetKey: p.TargetKey, TargetType: p.TargetType,
			ObservedAt: p.ObservedAt.UTC().Format(time.RFC3339Nano), CheckStatus: p.CheckStatus,
			StatusClass: p.StatusClass, ErrorClass: p.ErrorClass,
		})
	}
	c.JSON(200, out)
}

func (h *Monitor) trend(c *gin.Context) {
	ctx := c.Request.Context()
	window := durationQuery(c, "window", 24*time.Hour)
	buckets := intQuery(c, "buckets", 24, 168)
	now := h.clock.Now().UTC()
	var points []monitorpostgres.TrendPoint
	err := h.uow.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		var err error
		points, err = monitorpostgres.NewQueryRepo().Trend(ctx, tx, now, window, buckets)
		return err
	})
	if err != nil {
		writeErr(c, err)
		return
	}
	out := make([]dto.MonitorTrendPoint, 0, len(points))
	for _, p := range points {
		out = append(out, dto.MonitorTrendPoint{
			BucketStart: p.Bucket.UTC().Format(time.RFC3339), Success: p.Success, Failure: p.Failure,
		})
	}
	c.JSON(200, out)
}

func (h *Monitor) stats(c *gin.Context) {
	ctx := c.Request.Context()
	window := durationQuery(c, "window", 24*time.Hour)
	var stats monitorpostgres.MonitorStats
	err := h.uow.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		var err error
		stats, err = monitorpostgres.NewQueryRepo().Stats(ctx, tx)
		return err
	})
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(200, dto.Stats{
		Window: window.String(), GeneratedAt: h.clock.Now().UTC().Format(time.RFC3339),
		Services: stats.Services, Targets: stats.Targets,
	})
}

func writeErr(c *gin.Context, err error) {
	code := apperr.Code(err)
	c.JSON(apperr.HTTPStatus(code), map[string]string{"code": code, "message": err.Error()})
}

func intQuery(c *gin.Context, key string, fallback, max int) int {
	raw := strings.TrimSpace(c.Query(key))
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return fallback
	}
	if max > 0 && n > max {
		return max
	}
	return n
}

func int64Query(c *gin.Context, key string, fallback int64) int64 {
	raw := strings.TrimSpace(c.Query(key))
	if raw == "" {
		return fallback
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n < 0 {
		return fallback
	}
	return n
}

func durationQuery(c *gin.Context, key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(c.Query(key))
	if raw == "" {
		return fallback
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return fallback
	}
	return d
}
