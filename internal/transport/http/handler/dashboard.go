package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	appdashboard "github.com/ecakeman/alert/internal/application/dashboard"
	"github.com/ecakeman/alert/internal/transport/http/dto"
	"github.com/ecakeman/alert/internal/transport/http/response"
)

// Dashboard 承载看板只读聚合（B4.1：路由 1）。
type Dashboard struct{ uc *appdashboard.Service }

func NewDashboard(uc *appdashboard.Service) *Dashboard { return &Dashboard{uc: uc} }

func (h *Dashboard) Summary(c *gin.Context) {
	sum, err := h.uc.Summary(c.Request.Context())
	if err != nil {
		writeErr(c, err)
		return
	}
	resp := dto.DashboardSummary{
		GeneratedAt:    sum.GeneratedAt.UTC().Format(time.RFC3339),
		Services:       sum.Services,
		Targets:        sum.Targets,
		Episodes:       sum.Episodes,
		RecentEpisodes: []dto.EpisodeSummary{},
		MonitorTrend:   []dto.MonitorTrendPoint{},
	}
	response.JSON(c, http.StatusOK, resp)
}
