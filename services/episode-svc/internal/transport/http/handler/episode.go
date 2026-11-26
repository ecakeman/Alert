package handler

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ecakeman/alert/services/episode-svc/internal/application/apperr"
	episodeapp "github.com/ecakeman/alert/services/episode-svc/internal/application/episode"
	portepisode "github.com/ecakeman/alert/services/episode-svc/internal/application/port/episode"
	"github.com/ecakeman/alert/services/episode-svc/internal/transport/http/dto"
)

// Episode 承载 P4.3 的三个用例入口；P4.6 再补齐 11 条域端点。
type Episode struct {
	svc   *episodeapp.Service
	clock interface{ Now() time.Time }
}

func NewEpisode(svc *episodeapp.Service, clock interface{ Now() time.Time }) *Episode {
	return &Episode{svc: svc, clock: clock}
}

// Register 注册 episode 域 API。
func (h *Episode) Register(router *gin.Engine) {
	g := router.Group("/internal")
	g.POST("/services/:service_id/episodes/open", h.openOrReuse)
	g.POST("/services/:service_id/app-events", h.appendAppEvent)
	g.POST("/episodes/:episode_id/input-versions", h.appendInputVersion)
}

func (h *Episode) openOrReuse(c *gin.Context) {
	ep, created, err := h.svc.OpenOrReuse(c.Request.Context(), c.Param("service_id"))
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(200, dto.OpenOrReuseOut{Episode: toEpisodeOut(ep), Created: created})
}

func (h *Episode) appendAppEvent(c *gin.Context) {
	var in dto.AppEventIn
	if err := c.ShouldBindJSON(&in); err != nil {
		writeErr(c, apperr.Validation("invalid request body"))
		return
	}
	eventTime := h.clock.Now().UTC()
	if in.EventTime != "" {
		parsed, err := time.Parse(time.RFC3339Nano, in.EventTime)
		if err != nil {
			writeErr(c, apperr.Validation("event_time must be RFC3339"))
			return
		}
		eventTime = parsed.UTC()
	}
	res, err := h.svc.AppendAppEvent(c.Request.Context(), c.Param("service_id"), episodeapp.AppEventInput{
		ProducerID: in.ProducerID, EventID: in.EventID, EventKind: in.EventKind,
		HealthStatus: in.HealthStatus, Payload: in.Payload, EventTime: eventTime,
	})
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(200, dto.AppEventOut{
		EventRecordID: res.Event.EventRecordID, AppEventSeq: res.Event.AppEventSeq,
		EventID: res.Event.EventID, Appended: res.Appended,
	})
}

func (h *Episode) appendInputVersion(c *gin.Context) {
	appended, err := h.svc.AppendInputVersion(c.Request.Context(), c.Param("episode_id"))
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(200, dto.AppendInputOut{Appended: appended})
}

func toEpisodeOut(ep portepisode.Episode) dto.EpisodeOut {
	return dto.EpisodeOut{
		EpisodeID: ep.EpisodeID, ServiceID: ep.ServiceID, Status: ep.Status,
		StartedBy: ep.StartedBy, InputHash: ep.InputHash,
		InputVersion: ep.InputVersion, ProcessedInputVersion: ep.ProcessedInputVersion,
		FinalInputVersion: ep.FinalInputVersion,
		StartedAt:         dto.FormatTime(ep.StartedAt), UpdatedAt: dto.FormatTime(ep.UpdatedAt),
	}
}

func writeErr(c *gin.Context, err error) {
	code := apperr.Code(err)
	c.JSON(apperr.HTTPStatus(code), map[string]string{"code": code, "message": err.Error()})
}