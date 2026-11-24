package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	appepisode "github.com/ecakeman/alert/internal/application/episode"
	"github.com/ecakeman/alert/internal/application/port"
	"github.com/ecakeman/alert/internal/transport/http/dto"
	"github.com/ecakeman/alert/internal/transport/http/response"
)

// Episode 承载故障读取与事件注入（B4.3：路由 11/13/14）。
type Episode struct{ uc *appepisode.Service }

func NewEpisode(uc *appepisode.Service) *Episode { return &Episode{uc: uc} }

func toEpisodeView(v appepisode.EpisodeView) dto.EpisodeView {
	monitor, app := "UNKNOWN", "UNKNOWN"
	for _, st := range v.Sources {
		switch st.SourceType {
		case "MONITOR":
			monitor = st.Status
		case "APP":
			app = st.Status
		}
	}
	ep := v.Episode
	name := v.ServiceName
	out := dto.EpisodeView{
		EpisodeID: ep.EpisodeID, ServiceID: ep.ServiceID, ServiceName: &name,
		Status: ep.Status, ResolutionPending: ep.ResolutionPending, StartedBy: ep.StartedBy,
		StartedAt:  ep.StartedAt.UTC().Format(time.RFC3339),
		ResolvedAt: fmtTime(ep.ResolvedAt), ArchivedAt: fmtTime(ep.ArchivedAt),
		InputHash: ep.InputHash, InputVersion: ep.InputVersion,
		ProcessedInputVersion: ep.ProcessedInputVersion, FinalInputVersion: ep.FinalInputVersion,
		MonitorStatus: monitor, AppStatus: app,
	}
	if v.ActiveTask != nil {
		out.ActiveTaskID = v.ActiveTask.TaskID
		out.ActiveTaskStatus = v.ActiveTask.Status
		out.ActiveTaskType = v.ActiveTask.TaskType
	}
	return out
}

// Get：GET /episodes/:episode_id
func (h *Episode) Get(c *gin.Context) {
	view, err := h.uc.Get(c.Request.Context(), c.Param("episode_id"))
	if err != nil {
		writeErr(c, err)
		return
	}
	response.JSON(c, http.StatusOK, toEpisodeView(view))
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
	ev := res.Event
	response.JSON(c, status, dto.AppEvent{
		EventRecordID: strconv.FormatInt(ev.EventRecordID, 10),
		EventID:       ev.EventID, EventKind: ev.EventKind, HealthStatus: ev.HealthStatus,
		EventTime:  ev.EventTime.UTC().Format(time.RFC3339),
		ReceivedAt: ev.ReceivedAt.UTC().Format(time.RFC3339),
		Payload:    json.RawMessage(ev.Payload),
	})
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
	items, err := h.uc.ListSummaries(c.Request.Context(), port.EpisodeFilter{
		Status: status, From: from, To: to, Limit: limit, Offset: offset,
	})
	if err != nil {
		writeErr(c, err)
		return
	}
	page := dto.EpisodesPage{Total: len(items), Limit: limit, Offset: offset, Episodes: make([]dto.EpisodeSummary, 0, len(items))}
	for _, it := range items {
		page.Episodes = append(page.Episodes, toEpisodeSummary(it))
	}
	response.JSON(c, http.StatusOK, page)
}

func toEpisodeSummary(d appepisode.EpisodeSummaryData) dto.EpisodeSummary {
	ep := d.Episode
	out := dto.EpisodeSummary{
		EpisodeID: ep.EpisodeID, ServiceID: ep.ServiceID, ServiceName: d.ServiceName,
		Status: ep.Status, ResolutionPending: ep.ResolutionPending, StartedBy: ep.StartedBy,
		StartedAt:  ep.StartedAt.UTC().Format(time.RFC3339),
		ResolvedAt: fmtTime(ep.ResolvedAt), ArchivedAt: fmtTime(ep.ArchivedAt),
		InputVersion: ep.InputVersion, ProcessedInputVersion: ep.ProcessedInputVersion,
		ActiveTaskStatus: d.ActiveTaskStatus, UpdatedAt: ep.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if d.ActiveTaskType != "" {
		at := d.ActiveTaskType
		out.ActiveTaskType = &at
	}
	if d.HasDiagnosisReport {
		has := true
		out.HasDiagnosisReport = &has
	}
	return out
}
func toMonitorFields(fields []appepisode.MonitorFieldView) []dto.MonitorField {
	out := make([]dto.MonitorField, 0, len(fields))
	for _, f := range fields {
		out = append(out, dto.MonitorField{
			TargetKey: f.TargetKey, TargetType: f.TargetType, Status: f.Status,
			StatusClass: f.StatusClass, ErrorClass: f.ErrorClass, StateFingerprint: f.StateFingerprint,
		})
	}
	return out
}

func toMonitorPoints(ps []port.MonitorPoint) []dto.MonitorPoint {
	out := make([]dto.MonitorPoint, 0, len(ps))
	for _, p := range ps {
		out = append(out, dto.MonitorPoint{
			PointSeq: p.PointSeq, TargetID: p.TargetID, TargetKey: p.TargetKey,
			TargetType: p.TargetType, ObservedAt: p.ObservedAt.UTC().Format(time.RFC3339),
			CheckStatus: p.CheckStatus, StatusClass: p.StatusClass, ErrorClass: p.ErrorClass,
		})
	}
	return out
}

func toAppEvents(es []port.AppEvent) []dto.AppEvent {
	out := make([]dto.AppEvent, 0, len(es))
	for _, e := range es {
		out = append(out, dto.AppEvent{
			EventRecordID: strconv.FormatInt(e.EventRecordID, 10), EventID: e.EventID,
			EventKind: e.EventKind, HealthStatus: e.HealthStatus,
			EventTime:  e.EventTime.UTC().Format(time.RFC3339),
			ReceivedAt: e.ReceivedAt.UTC().Format(time.RFC3339),
			Payload:    json.RawMessage(e.Payload),
		})
	}
	return out
}

func toArtifactRefs(as []port.ArtifactRef) []dto.ArtifactRef {
	out := make([]dto.ArtifactRef, 0, len(as))
	for _, a := range as {
		out = append(out, dto.ArtifactRef{
			ArtifactType: a.ArtifactType, ArtifactVersion: a.ArtifactVersion,
			StorageKey: a.StorageKey, ContentType: a.ContentType,
			ContentHash: a.ContentHash, UpdatedAt: a.UpdatedAt.UTC().Format(time.RFC3339),
		})
	}
	return out
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
		Episode:              toEpisodeView(d.Episode),
		Region1Projection:    toMonitorFields(d.Projection),
		MonitorStartPointSeq: d.MonitorStartSeq,
		MonitorPoints:        toMonitorPoints(d.MonitorPoints),
		Region2Events:        toAppEvents(d.AppEvents),
		Region3Refs:          toArtifactRefs(d.Artifacts),
	})
}
