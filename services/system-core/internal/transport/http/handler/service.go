package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	portService "github.com/ecakeman/alert/services/system-core/internal/application/port/service"
	appservice "github.com/ecakeman/alert/services/system-core/internal/application/service"
	"github.com/ecakeman/alert/services/system-core/internal/transport/http/dto"
	"github.com/ecakeman/alert/services/system-core/internal/transport/http/response"
)

// Service 承载服务相关的 HTTP Handler（B4.1：路由 2~6）。
type Service struct{ uc *appservice.Service }

func NewService(uc *appservice.Service) *Service { return &Service{uc: uc} }

func toServiceSummary(s appservice.ServiceSummaryData) dto.ServiceSummary {
	return dto.ServiceSummary{
		ServiceID: s.Service.ServiceID, ServiceKey: s.Service.ServiceKey, Name: s.Service.Name,
		Description: s.Service.Description, Enabled: s.Service.Enabled,
		MonitorStatus: s.MonitorStatus, AppStatus: s.AppStatus, TargetCount: s.TargetCount,
		LastCheckAt: fmtTime(s.LastCheckAt), ActiveEpisodeID: s.ActiveEpisodeID,
	}
}

func toServiceView(v appservice.ServiceViewData) dto.ServiceView {
	targets := make([]dto.Target, 0, len(v.Targets))
	for _, t := range v.Targets {
		targets = append(targets, toTarget(t))
	}
	s := v.Summary
	return dto.ServiceView{
		ServiceID: s.Service.ServiceID, ServiceKey: s.Service.ServiceKey, Name: s.Service.Name,
		Description: s.Service.Description, Enabled: s.Service.Enabled,
		MonitorAggregationMode: s.Service.MonitorAggregationMode,
		AuthorityProducerID:    s.Service.AuthorityProducerID,
		MonitorStatus:          s.MonitorStatus, AppStatus: s.AppStatus, Targets: targets,
		TargetCount: s.TargetCount, LastCheckAt: fmtTime(s.LastCheckAt), ActiveEpisodeID: s.ActiveEpisodeID,
	}
}

// List：GET /services
func (h *Service) List(c *gin.Context) {
	limit, ok := parseLimit(c.Query("limit"), 50)
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
	case "", "NORMAL", "ABNORMAL", "UNKNOWN":
	default:
		response.Error(c, http.StatusBadRequest, "VALIDATION", "invalid status")
		return
	}
	page, err := h.uc.ListSummaries(c.Request.Context(), portService.ServiceFilter{
		Query: c.Query("q"), Status: status, Limit: limit, Offset: offset,
	})
	if err != nil {
		writeErr(c, err)
		return
	}
	resp := dto.ServicesPage{Total: page.Total, Limit: page.Limit, Offset: page.Offset, Services: make([]dto.ServiceSummary, 0, len(page.Items))}
	for _, s := range page.Items {
		resp.Services = append(resp.Services, toServiceSummary(s))
	}
	response.JSON(c, http.StatusOK, resp)
}

// Create：POST /services
func (h *Service) Create(c *gin.Context) {
	var in dto.ServiceCreatePayload
	if err := c.ShouldBindJSON(&in); err != nil {
		response.Error(c, http.StatusBadRequest, "VALIDATION", "invalid json body")
		return
	}
	targets := make([]appservice.TargetInput, 0, len(in.Targets))
	for _, t := range in.Targets {
		targets = append(targets, toTargetInput(t))
	}
	view, err := h.uc.Create(c.Request.Context(), appservice.CreateServiceInput{
		ServiceKey: in.ServiceKey, Name: in.Name, Description: in.Description,
		AuthorityProducerID: in.AuthorityProducerID, Targets: targets,
	})
	if err != nil {
		writeErr(c, err)
		return
	}
	full, err := h.uc.GetView(c.Request.Context(), view.Service.ServiceID)
	if err != nil {
		writeErr(c, err)
		return
	}
	response.JSON(c, http.StatusCreated, toServiceView(full))
}

// Get：GET /services/:service_id
func (h *Service) Get(c *gin.Context) {
	full, err := h.uc.GetView(c.Request.Context(), c.Param("service_id"))
	if err != nil {
		writeErr(c, err)
		return
	}
	response.JSON(c, http.StatusOK, toServiceView(full))
}

// Patch：PATCH /services/:service_id
func (h *Service) Patch(c *gin.Context) {
	var p dto.ServicePatchPayload
	if err := c.ShouldBindJSON(&p); err != nil {
		response.Error(c, http.StatusBadRequest, "VALIDATION", "invalid json body")
		return
	}
	if _, err := h.uc.Patch(c.Request.Context(), c.Param("service_id"), portService.ServicePatch{
		Name: p.Name, Description: p.Description, Enabled: p.Enabled,
	}); err != nil {
		writeErr(c, err)
		return
	}
	full, err := h.uc.GetView(c.Request.Context(), c.Param("service_id"))
	if err != nil {
		writeErr(c, err)
		return
	}
	response.JSON(c, http.StatusOK, toServiceView(full))
}

// Delete：DELETE /services/:service_id
func (h *Service) Delete(c *gin.Context) {
	if err := h.uc.SoftDelete(c.Request.Context(), c.Param("service_id")); err != nil {
		writeErr(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
