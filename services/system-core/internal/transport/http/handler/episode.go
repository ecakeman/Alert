package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	appepisode "github.com/ecakeman/alert/services/system-core/internal/application/episode"
	portepisode "github.com/ecakeman/alert/services/system-core/internal/application/port/episode"
	"github.com/ecakeman/alert/services/system-core/internal/transport/http/dto"
	"github.com/ecakeman/alert/services/system-core/internal/transport/http/handler/mapper"
	"github.com/ecakeman/alert/services/system-core/internal/transport/http/response"
)

// Episode 承载故障读取与事件注入（B4.3：路由 11/13/14）。
type Episode struct{ uc *appepisode.Service }

func NewEpisode(uc *appepisode.Service) *Episode { return &Episode{uc: uc} }

// Get：GET /episodes/:episode_id
func (h *Episode) Get(c *gin.Context) {
	view, err := h.uc.Get(c.Request.Context(), c.Param("episode_id"))
	if err != nil {
		writeErr(c, err)
		return
	}
	response.JSON(c, http.StatusOK, mapper.ToEpisodeView(view))
}

// AppendEvent：POST /services/:service_id/events
func (h *Episode) AppendEvent(c *gin.Context) {
	var in dto.AppEventInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.Error(c, http.StatusBadRequest, "VALIDATION", "invalid json body")
		return
	}
	when := time.Now().UTC()
	if in.EventTime != "" {
		t, err := time.Parse(time.RFC3339, in.EventTime)
		if err != nil {
			response.Error(c, http.StatusBadRequest, "VALIDATION", "invalid event_time")
			return
		}
		when = t.UTC()
	}
	var payload []byte
	if len(in.Payload) > 0 {
		payload = []byte(in.Payload)
	}
	res, err := h.uc.AppendAppEvent(c.Request.Context(), c.Param("service_id"), appepisode.AppEventInput{
		ProducerID: in.ProducerID, EventID: in.EventID, EventKind: in.EventKind,
		HealthStatus: in.HealthStatus, Payload: payload, EventTime: when,
	})
	if err != nil {
		writeErr(c, err)
		return
	}
	status := http.StatusOK
	if res.Appended {
		status = http.StatusCreated
	}
	response.JSON(c, status, mapper.ToAppEvent(res.Event))
}

// List：GET /episodes
func (h *Episode) List(c *gin.Context) {
	limit, ok := parseLimit(c.Query("limit"), 20)
	if !ok {
		response.Error(c, http.StatusBadRequest, "VALIDATION", "invalid limit")
		return
	}
	offset, ok := parseOffset(c.Query("offset"))
	if !ok {
		response.Error(c, http.StatusBadRequest, "VALIDATION", "invalid offset")
		return
	}
	status := c.Query("status")
	switch status {
	case "", "ACTIVE", "RESOLVED", "ARCHIVED":
	default:
		response.Error(c, http.StatusBadRequest, "VALIDATION", "invalid status")
		return
	}
	var from, to *time.Time
	if v := c.Query("from"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			response.Error(c, http.StatusBadRequest, "VALIDATION", "invalid from")
			return
		}
		from = &t
	}
	if v := c.Query("to"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			response.Error(c, http.StatusBadRequest, "VALIDATION", "invalid to")
			return
		}
		to = &t
	}
	items, err := h.uc.ListSummaries(c.Request.Context(), portepisode.EpisodeFilter{
		Status: status, From: from, To: to, Limit: limit, Offset: offset,
	})
	if err != nil {
		writeErr(c, err)
		return
	}
	page := dto.EpisodesPage{Total: len(items), Limit: limit, Offset: offset, Episodes: make([]dto.EpisodeSummary, 0, len(items))}
	for _, it := range items {
		page.Episodes = append(page.Episodes, mapper.ToEpisodeSummary(it))
	}
	response.JSON(c, http.StatusOK, page)
}

// ServiceTimeline：GET /services/:service_id/timeline
func (h *Episode) ServiceTimeline(c *gin.Context) {
	limit, ok := parseLimit(c.Query("limit"), 100)
	if !ok {
		response.Error(c, http.StatusBadRequest, "VALIDATION", "invalid limit")
		return
	}
	recs, err := h.uc.ServiceTimeline(c.Request.Context(), c.Param("service_id"), limit)
	if err != nil {
		writeErr(c, err)
		return
	}
	out := dto.ServiceTimeline{Records: make([]dto.ServiceTimelineRecord, 0, len(recs))}
	for _, r := range recs {
		out.Records = append(out.Records, dto.ServiceTimelineRecord{
			RecordType: r.RecordType, RecordID: r.RecordID,
			EventTime:  r.EventTime.UTC().Format(time.RFC3339),
			ReceivedAt: r.ReceivedAt.UTC().Format(time.RFC3339),
			Source:     r.Source, Content: json.RawMessage(r.Content),
		})
	}
	response.JSON(c, http.StatusOK, out)
}

// Timeline：GET /episodes/:episode_id/timeline
func (h *Episode) Timeline(c *gin.Context) {
	d, err := h.uc.Timeline(c.Request.Context(), c.Param("episode_id"))
	if err != nil {
		writeErr(c, err)
		return
	}
	response.JSON(c, http.StatusOK, dto.EpisodeTimeline{
		Episode:              mapper.ToEpisodeView(d.Episode),
		Region1Projection:    mapper.ToMonitorFields(d.Projection),
		MonitorStartPointSeq: d.MonitorStartSeq,
		MonitorPoints:        mapper.ToMonitorPoints(d.MonitorPoints),
		Region2Events:        mapper.ToAppEvents(d.AppEvents),
		Region3Refs:          mapper.ToArtifactRefs(d.Artifacts),
	})
}
