package handler

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ecakeman/alert/services/contracts"
	"github.com/ecakeman/alert/services/episode-svc/internal/application/apperr"
	episodeapp "github.com/ecakeman/alert/services/episode-svc/internal/application/episode"
	portepisode "github.com/ecakeman/alert/services/episode-svc/internal/application/port/episode"
	"github.com/ecakeman/alert/services/episode-svc/internal/transport/http/dto"
)

func (h *Episode) listEpisodes(c *gin.Context) {
	f := portepisode.EpisodeFilter{
		ServiceID: strings.TrimSpace(c.Query("service_id")),
		Status:    strings.ToUpper(strings.TrimSpace(c.Query("status"))),
		Limit:     intQuery(c, "limit", 20, 200),
		Offset:    intQuery(c, "offset", 0, 1<<30),
	}
	page, err := h.svc.ListEpisodes(c.Request.Context(), f)
	if err != nil {
		writeErr(c, err)
		return
	}
	out := contracts.EpisodesPage{Total: page.Total, Limit: page.Limit, Offset: page.Offset,
		Episodes: make([]contracts.EpisodeSummary, 0, len(page.Episodes))}
	for _, ep := range page.Episodes {
		out.Episodes = append(out.Episodes, toSummary(ep))
	}
	c.JSON(200, out)
}

// getEpisode 同时承载详情与两个保留字批量端点（避免 gin 静态/参数路由冲突）。
func (h *Episode) getEpisode(c *gin.Context) {
	switch c.Param("episode_id") {
	case "active-by-services":
		h.activeByServices(c)
		return
	case "pending-dispatch":
		h.pendingDispatch(c)
		return
	}
	ep, err := h.svc.GetEpisode(c.Request.Context(), c.Param("episode_id"))
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(200, toView(ep))
}

func (h *Episode) activeByServices(c *gin.Context) {
	ids := splitCSV(c.Query("service_ids"))
	if len(ids) == 0 {
		writeErr(c, apperr.Validation("service_ids is required"))
		return
	}
	out, err := h.svc.ActiveByServices(c.Request.Context(), ids)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(200, out)
}

func (h *Episode) pendingDispatch(c *gin.Context) {
	out, err := h.svc.PendingDispatch(c.Request.Context(), intQuery(c, "limit", 64, 64))
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(200, out)
}

func (h *Episode) appEvents(c *gin.Context) {
	events, err := h.svc.ServiceAppEvents(c.Request.Context(), c.Param("service_id"), intQuery(c, "limit", 100, 500))
	if err != nil {
		writeErr(c, err)
		return
	}
	out := make([]contracts.AppEvent, 0, len(events))
	for _, e := range events {
		out = append(out, toAppEvent(e))
	}
	c.JSON(200, out)
}

func (h *Episode) timelineParts(c *gin.Context) {
	parts, err := h.svc.TimelineParts(c.Request.Context(), c.Param("episode_id"), intQuery(c, "limit", 200, 500))
	if err != nil {
		writeErr(c, err)
		return
	}
	projection := json.RawMessage(parts.InputVersion.MonitorProjection)
	if len(projection) == 0 {
		projection = json.RawMessage("[]")
	}
	events := make([]contracts.AppEvent, 0, len(parts.AppEvents))
	for _, e := range parts.AppEvents {
		events = append(events, toAppEvent(e))
	}
	c.JSON(200, gin.H{
		"episode":           toView(parts.Episode),
		"projection":        projection,
		"monitor_start_seq": parts.MonitorStartSeq,
		"app_events":        events,
	})
}

func (h *Episode) stats(c *gin.Context) {
	st, err := h.svc.Stats(c.Request.Context())
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(200, st)
}

func toSummary(ep portepisode.Episode) contracts.EpisodeSummary {
	out := contracts.EpisodeSummary{
		EpisodeID: ep.EpisodeID, ServiceID: ep.ServiceID, Status: ep.Status,
		ResolutionPending: ep.ResolutionPending, StartedBy: ep.StartedBy,
		StartedAt: dto.FormatTime(ep.StartedAt), InputVersion: ep.InputVersion,
		ProcessedInputVersion: ep.ProcessedInputVersion, UpdatedAt: dto.FormatTime(ep.UpdatedAt),
	}
	if ep.ResolvedAt != nil {
		s := dto.FormatTime(*ep.ResolvedAt)
		out.ResolvedAt = &s
	}
	if ep.ArchivedAt != nil {
		s := dto.FormatTime(*ep.ArchivedAt)
		out.ArchivedAt = &s
	}
	return out
}

func toView(ep portepisode.Episode) contracts.EpisodeView {
	out := contracts.EpisodeView{
		EpisodeID: ep.EpisodeID, ServiceID: ep.ServiceID, Status: ep.Status,
		ResolutionPending: ep.ResolutionPending, StartedBy: ep.StartedBy,
		StartedAt: dto.FormatTime(ep.StartedAt), InputHash: ep.InputHash,
		InputVersion: ep.InputVersion, ProcessedInputVersion: ep.ProcessedInputVersion,
		FinalInputVersion: ep.FinalInputVersion,
	}
	if !ep.UpdatedAt.IsZero() {
		s := dto.FormatTime(ep.UpdatedAt)
		out.UpdatedAt = &s
	}
	if ep.ResolvedAt != nil {
		s := dto.FormatTime(*ep.ResolvedAt)
		out.ResolvedAt = &s
	}
	if ep.ArchivedAt != nil {
		s := dto.FormatTime(*ep.ArchivedAt)
		out.ArchivedAt = &s
	}
	return out
}

func toAppEvent(e portepisode.AppEvent) contracts.AppEvent {
	return contracts.AppEvent{
		EventRecordID: strconv.FormatInt(e.EventRecordID, 10),
		EventID:       e.EventID, EventKind: e.EventKind, HealthStatus: e.HealthStatus,
		EventTime: dto.FormatTime(e.EventTime), ReceivedAt: dto.FormatTime(e.ReceivedAt),
		Payload: json.RawMessage(e.Payload),
	}
}

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
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

var _ = episodeapp.EpisodePage{}