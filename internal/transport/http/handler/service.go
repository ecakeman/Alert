package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ecakeman/alert/internal/application/apperr"
	portService "github.com/ecakeman/alert/internal/application/port/service"
	appservice "github.com/ecakeman/alert/internal/application/service"
	"github.com/ecakeman/alert/internal/transport/http/dto"
	"github.com/ecakeman/alert/internal/transport/http/httperr"
	"github.com/ecakeman/alert/internal/transport/http/response"
)

// Service 承载服务与目标相关的 HTTP Handler（B4.1：路由 2~6）。
type Service struct{ uc *appservice.Service }

func NewService(uc *appservice.Service) *Service { return &Service{uc: uc} }

func writeErr(c *gin.Context, err error) {
	status, code, msg := httperr.Classify(apperr.Normalize(err))
	response.Error(c, status, code, msg)
}

func parseLimit(raw string, def int) (int, bool) {
	if raw == "" {
		return def, true
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 || n > 200 {
		return 0, false
	}
	return n, true
}

func parseOffset(raw string) (int, bool) {
	if raw == "" {
		return 0, true
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

func fmtTime(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.UTC().Format(time.RFC3339)
	return &s
}

func toTarget(t portService.MonitorTarget) dto.Target {
	cfg := json.RawMessage("null")
	if len(t.Config) > 0 {
		cfg = json.RawMessage(t.Config)
	}
	return dto.Target{
		TargetID: t.TargetID, TargetKey: t.TargetKey, TargetName: t.TargetName,
		TargetType: t.TargetType, Address: t.Address, Config: cfg,
		IntervalMS: t.IntervalMS, TimeoutMS: t.TimeoutMS,
		FailureThreshold: t.FailureThreshold, RecoveryThreshold: t.RecoveryThreshold,
		IsRequired: t.IsRequired, Enabled: t.Enabled, Status: t.Status,
		ConsecutiveFailures: t.ConsecutiveFailures, ConsecutiveSuccesses: t.ConsecutiveSuccesses,
		LastCheckAt: fmtTime(t.LastCheckAt),
	}
}

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

func toTargetInput(t dto.TargetInput) appservice.TargetInput {
	var cfg []byte
	if len(t.Config) > 0 {
		cfg = []byte(t.Config)
	}
	return appservice.TargetInput{
		TargetKey: t.TargetKey, TargetName: t.TargetName, Address: t.Address,
		TargetType: t.TargetType, Config: cfg, IntervalMS: t.IntervalMS, TimeoutMS: t.TimeoutMS,
		FailureThreshold: t.FailureThreshold, RecoveryThreshold: t.RecoveryThreshold,
		IsRequired: t.IsRequired,
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
func toTargetPatch(p dto.TargetPatchPayload) portService.TargetPatch {
	var cfg []byte
	if p.Config != nil {
		cfg = []byte(*p.Config)
	}
	return portService.TargetPatch{
		TargetName: p.TargetName, Address: p.Address, Config: cfg,
		IntervalMS: p.IntervalMS, TimeoutMS: p.TimeoutMS,
		FailureThreshold: p.FailureThreshold, RecoveryThreshold: p.RecoveryThreshold,
		IsRequired: p.IsRequired, Enabled: p.Enabled,
	}
}

// ListTargets：GET /services/:service_id/targets
func (h *Service) ListTargets(c *gin.Context) {
	targets, err := h.uc.ListTargets(c.Request.Context(), c.Param("service_id"))
	if err != nil {
		writeErr(c, err)
		return
	}
	resp := dto.TargetsResponse{Targets: make([]dto.Target, 0, len(targets))}
	for _, t := range targets {
		resp.Targets = append(resp.Targets, toTarget(t))
	}
	response.JSON(c, http.StatusOK, resp)
}

// CreateTarget：POST /services/:service_id/targets
func (h *Service) CreateTarget(c *gin.Context) {
	var in dto.TargetInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.Error(c, http.StatusBadRequest, "VALIDATION", "invalid json body")
		return
	}
	t, err := h.uc.CreateTarget(c.Request.Context(), c.Param("service_id"), toTargetInput(in))
	if err != nil {
		writeErr(c, err)
		return
	}
	response.JSON(c, http.StatusCreated, toTarget(t))
}

// PatchTarget：PATCH /targets/:target_id
func (h *Service) PatchTarget(c *gin.Context) {
	var p dto.TargetPatchPayload
	if err := c.ShouldBindJSON(&p); err != nil {
		response.Error(c, http.StatusBadRequest, "VALIDATION", "invalid json body")
		return
	}
	t, err := h.uc.PatchTarget(c.Request.Context(), c.Param("target_id"), toTargetPatch(p))
	if err != nil {
		writeErr(c, err)
		return
	}
	response.JSON(c, http.StatusOK, toTarget(t))
}

// DeleteTarget：DELETE /targets/:target_id
func (h *Service) DeleteTarget(c *gin.Context) {
	if err := h.uc.DeleteTarget(c.Request.Context(), c.Param("target_id")); err != nil {
		writeErr(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
