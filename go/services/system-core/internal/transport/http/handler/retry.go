package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	apptask "github.com/ecakeman/alert/services/system-core/internal/application/task"
	"github.com/ecakeman/alert/services/system-core/internal/transport/http/handler/mapper"
	"github.com/ecakeman/alert/services/system-core/internal/transport/http/response"
)

// Retry 承载路由 17/18：异步受理人工诊断/归档重试。
type Retry struct{ uc *apptask.Service }

func NewRetry(uc *apptask.Service) *Retry { return &Retry{uc: uc} }

func (h *Retry) RetryDiagnosis(c *gin.Context) {
	view, err := h.uc.RetryDiagnosis(c.Request.Context(), c.Param("episode_id"))
	if err != nil {
		writeErr(c, err)
		return
	}
	response.JSON(c, http.StatusAccepted, mapper.ToTaskAccepted(view, false))
}

func (h *Retry) RetryArchive(c *gin.Context) {
	view, err := h.uc.RetryArchive(c.Request.Context(), c.Param("episode_id"))
	if err != nil {
		writeErr(c, err)
		return
	}
	response.JSON(c, http.StatusAccepted, mapper.ToTaskAccepted(view, false))
}
