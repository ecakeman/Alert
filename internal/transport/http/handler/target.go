package handler

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"

	portService "github.com/ecakeman/alert/internal/application/port/service"
	appservice "github.com/ecakeman/alert/internal/application/service"
	"github.com/ecakeman/alert/internal/transport/http/dto"
	"github.com/ecakeman/alert/internal/transport/http/response"
)

// Target 承载目标相关的 HTTP Handler（B4.1：路由 7~10）。
type Target struct{ uc *appservice.Service }

func NewTarget(uc *appservice.Service) *Target { return &Target{uc: uc} }

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
func (h *Target) ListTargets(c *gin.Context) {
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
func (h *Target) CreateTarget(c *gin.Context) {
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
func (h *Target) PatchTarget(c *gin.Context) {
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
func (h *Target) DeleteTarget(c *gin.Context) {
	if err := h.uc.DeleteTarget(c.Request.Context(), c.Param("target_id")); err != nil {
		writeErr(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
