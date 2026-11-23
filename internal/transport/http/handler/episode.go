package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	appepisode "github.com/ecakeman/alert/internal/application/episode"
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
