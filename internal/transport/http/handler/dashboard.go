package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	appdashboard "github.com/ecakeman/alert/internal/application/dashboard"
	portdashboard "github.com/ecakeman/alert/internal/application/port/dashboard"
	"github.com/ecakeman/alert/internal/transport/http/dto"
	"github.com/ecakeman/alert/internal/transport/http/response"
)

// Dashboard 承载看板只读聚合（B4.1：路由 1）。
type Dashboard struct{ uc *appdashboard.Service }

func NewDashboard(uc *appdashboard.Service) *Dashboard { return &Dashboard{uc: uc} }

func toRecentEpisodeSummary(r portdashboard.RecentEpisode) dto.EpisodeSummary {
	out := dto.EpisodeSummary{
		EpisodeID: r.EpisodeID, ServiceID: r.ServiceID, ServiceName: r.ServiceName,
		Status: r.Status, ResolutionPending: r.ResolutionPending, StartedBy: r.StartedBy,
		StartedAt:  r.StartedAt.UTC().Format(time.RFC3339),
		ResolvedAt: fmtTime(r.ResolvedAt), ArchivedAt: fmtTime(r.ArchivedAt),
		InputVersion: r.InputVersion, ProcessedInputVersion: r.ProcessedInputVersion,
		ActiveTaskStatus: r.ActiveTaskStatus, UpdatedAt: r.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if r.ActiveTaskType != "" {
		at := r.ActiveTaskType
		out.ActiveTaskType = &at
	}
	has := r.HasDiagnosisReport
	out.HasDiagnosisReport = &has
	return out
}

// Summary：GET /dashboard/summary
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
		RecentEpisodes: make([]dto.EpisodeSummary, 0, len(sum.Recent)),
		MonitorTrend:   make([]dto.MonitorTrendPoint, 0, len(sum.Trend)),
	}
	for _, r := range sum.Recent {
		resp.RecentEpisodes = append(resp.RecentEpisodes, toRecentEpisodeSummary(r))
	}
	for _, p := range sum.Trend {
		resp.MonitorTrend = append(resp.MonitorTrend, dto.MonitorTrendPoint{
			BucketStart: p.Bucket.UTC().Format(time.RFC3339),
			Success:     p.Success,
			Failure:     p.Failure,
		})
	}
	response.JSON(c, http.StatusOK, resp)
}
