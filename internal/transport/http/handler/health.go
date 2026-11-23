package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ecakeman/alert/internal/application/health"
	"github.com/ecakeman/alert/internal/transport/http/response"
)

// Health 暴露存活与就绪探针。
type Health struct {
	svc *health.Service
}

func NewHealth(svc *health.Service) *Health { return &Health{svc: svc} }

// Live 存活探针：恒 200 ok。
func (h *Health) Live(c *gin.Context) {
	response.JSON(c, http.StatusOK, h.svc.Live())
}

// Ready 就绪探针：全部 checker 通过返回 200，否则 503。
func (h *Health) Ready(c *gin.Context) {
	st := h.svc.Ready(c.Request.Context())
	if st.Status != "ready" {
		response.Error(c, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "not ready")
		return
	}
	response.JSON(c, http.StatusOK, st)
}
