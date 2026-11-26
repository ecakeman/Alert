package handler

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ecakeman/alert/services/platform/apierr"

	"github.com/ecakeman/alert/services/service-registry/internal/application/apperr"
	portService "github.com/ecakeman/alert/services/service-registry/internal/application/port/service"
	appservice "github.com/ecakeman/alert/services/service-registry/internal/application/service"
	"github.com/ecakeman/alert/services/service-registry/internal/transport/http/dto"
)

const maxBatchIDs = 200

// Registry 承载 11 个 /internal 域端点。
type Registry struct {
	uc *appservice.Service
}

func NewRegistry(uc *appservice.Service) *Registry { return &Registry{uc: uc} }

// Register 注册 registry 域 API。
func (h *Registry) Register(router *gin.Engine) {
	g := router.Group("/internal")
	g.GET("/services", h.listServices)
	g.POST("/services", h.createService)
	g.GET("/services/:service_id", h.getService)
	g.PATCH("/services/:service_id", h.patchService)
	g.DELETE("/services/:service_id", h.deleteService)
	g.GET("/services/:service_id/targets", h.listTargets)
	g.POST("/services/:service_id/targets", h.createTarget)
	g.GET("/targets", h.listEnabledTargets)
	g.GET("/targets/:target_id", h.getTarget)
	g.POST("/targets/:target_id/probe-result", h.probeResult)
	g.PATCH("/targets/:target_id", h.patchTarget)
	g.DELETE("/targets/:target_id", h.deleteTarget)
	g.GET("/services/:service_id/source-states", h.listSourceStates)
	g.POST("/services/:service_id/source-states", h.upsertSourceState)
	g.GET("/stats", h.stats)
}

func (h *Registry) listServices(c *gin.Context) {
	ctx := c.Request.Context()

	if rawIDs := strings.TrimSpace(c.Query("ids")); rawIDs != "" {
		ids := splitCSV(rawIDs)
		if len(ids) == 0 || len(ids) > maxBatchIDs {
			writeErr(c, apperr.Validation("ids must contain 1..200 values"))
			return
		}
		services, err := h.uc.ListByIDs(ctx, ids)
		if err != nil {
			writeErr(c, err)
			return
		}
		summaries, err := h.summaries(ctx, services)
		if err != nil {
			writeErr(c, err)
			return
		}
		c.JSON(200, dto.ServicesPage{Total: len(summaries), Limit: len(summaries), Offset: 0, Services: summaries})
		return
	}

	limit := intQuery(c, "limit", 20, 200)
	offset := intQuery(c, "offset", 0, 1<<31-1)
	filter := portService.ServiceFilter{Limit: limit, Offset: offset}
	services, err := h.uc.List(ctx, filter)
	if err != nil {
		writeErr(c, err)
		return
	}
	total, err := h.uc.Count(ctx, filter)
	if err != nil {
		writeErr(c, err)
		return
	}
	summaries, err := h.summaries(ctx, services)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(200, dto.ServicesPage{Total: total, Limit: limit, Offset: offset, Services: summaries})
}

func (h *Registry) createService(c *gin.Context) {
	var payload dto.ServiceCreatePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		writeErr(c, apperr.Validation("invalid json body"))
		return
	}
	view, err := h.uc.Create(c.Request.Context(), appservice.CreateServiceInput{
		ServiceKey:          payload.ServiceKey,
		Name:                payload.Name,
		Description:         payload.Description,
		AuthorityProducerID: payload.AuthorityProducerID,
		Targets:             toAppTargetInputs(payload.Targets),
	})
	if err != nil {
		writeErr(c, err)
		return
	}
	out, err := h.toView(c.Request.Context(), view, true)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(201, out)
}

func (h *Registry) getService(c *gin.Context) {
	view, err := h.uc.Get(c.Request.Context(), c.Param("service_id"))
	if err != nil {
		writeErr(c, err)
		return
	}
	out, err := h.toView(c.Request.Context(), view, true)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(200, out)
}

func (h *Registry) patchService(c *gin.Context) {
	var payload dto.ServicePatchPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		writeErr(c, apperr.Validation("invalid json body"))
		return
	}
	view, err := h.uc.Patch(c.Request.Context(), c.Param("service_id"), portService.ServicePatch{
		Name:                   payload.Name,
		Description:            payload.Description,
		Enabled:                payload.Enabled,
		MonitorAggregationMode: payload.MonitorAggregationMode,
	})
	if err != nil {
		writeErr(c, err)
		return
	}
	out, err := h.toView(c.Request.Context(), view, true)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(200, out)
}

func (h *Registry) deleteService(c *gin.Context) {
	if err := h.uc.SoftDelete(c.Request.Context(), c.Param("service_id")); err != nil {
		writeErr(c, err)
		return
	}
	c.Status(204)
}

func (h *Registry) listTargets(c *gin.Context) {
	targets, err := h.uc.ListTargets(c.Request.Context(), c.Param("service_id"))
	if err != nil {
		writeErr(c, err)
		return
	}
	out := make([]dto.Target, 0, len(targets))
	for _, target := range targets {
		out = append(out, toTarget(target))
	}
	c.JSON(200, out)
}

func (h *Registry) createTarget(c *gin.Context) {
	var payload dto.TargetInput
	if err := c.ShouldBindJSON(&payload); err != nil {
		writeErr(c, apperr.Validation("invalid json body"))
		return
	}
	target, err := h.uc.CreateTarget(c.Request.Context(), c.Param("service_id"), toAppTargetInput(payload))
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(201, toTarget(target))
}

func (h *Registry) patchTarget(c *gin.Context) {
	var payload dto.TargetPatchPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		writeErr(c, apperr.Validation("invalid json body"))
		return
	}
	target, err := h.uc.PatchTarget(c.Request.Context(), c.Param("target_id"), portService.TargetPatch{
		TargetName:        payload.TargetName,
		Address:           payload.Address,
		Config:            rawMessageBytes(payload.Config),
		IntervalMS:        payload.IntervalMS,
		TimeoutMS:         payload.TimeoutMS,
		FailureThreshold:  payload.FailureThreshold,
		RecoveryThreshold: payload.RecoveryThreshold,
		IsRequired:        payload.IsRequired,
		Enabled:           payload.Enabled,
	})
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(200, toTarget(target))
}

func (h *Registry) deleteTarget(c *gin.Context) {
	if err := h.uc.DeleteTarget(c.Request.Context(), c.Param("target_id")); err != nil {
		writeErr(c, err)
		return
	}
	c.Status(204)
}

func (h *Registry) listSourceStates(c *gin.Context) {
	states, err := h.uc.ListSourceStates(c.Request.Context(), c.Param("service_id"))
	if err != nil {
		writeErr(c, err)
		return
	}
	out := make([]dto.SourceState, 0, len(states))
	for _, state := range states {
		out = append(out, dto.SourceState{
			ServiceID: state.ServiceID, SourceType: state.SourceType,
			Status: state.Status, Reason: state.Reason,
		})
	}
	c.JSON(200, out)
}

func (h *Registry) stats(c *gin.Context) {
	stats, err := h.uc.Stats(c.Request.Context())
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(200, dto.Stats{Services: stats.Services, Targets: stats.Targets})
}

func (h *Registry) summaries(ctx context.Context, services []portService.Service) ([]dto.ServiceSummary, error) {
	out := make([]dto.ServiceSummary, 0, len(services))
	for _, svc := range services {
		targets, err := h.uc.ListTargets(ctx, svc.ServiceID)
		if err != nil {
			return nil, err
		}
		states, err := h.uc.ListSourceStates(ctx, svc.ServiceID)
		if err != nil {
			return nil, err
		}
		monitor, app := sourceStatuses(states)
		out = append(out, dto.ServiceSummary{
			ServiceID: svc.ServiceID, ServiceKey: svc.ServiceKey, Name: svc.Name,
			Description: svc.Description, Enabled: svc.Enabled,
			MonitorStatus: monitor, AppStatus: app,
			TargetCount: len(targets), LastCheckAt: latestCheckAt(targets),
		})
	}
	return out, nil
}

func (h *Registry) toView(ctx context.Context, view appservice.ServiceView, withStates bool) (dto.ServiceView, error) {
	monitor, app := "UNKNOWN", "UNKNOWN"
	if withStates {
		states, err := h.uc.ListSourceStates(ctx, view.Service.ServiceID)
		if err != nil {
			return dto.ServiceView{}, err
		}
		monitor, app = sourceStatuses(states)
	}
	out := dto.ServiceView{
		ServiceID:              view.Service.ServiceID,
		ServiceKey:             view.Service.ServiceKey,
		Name:                   view.Service.Name,
		Description:            view.Service.Description,
		Enabled:                view.Service.Enabled,
		MonitorAggregationMode: view.Service.MonitorAggregationMode,
		AuthorityProducerID:    view.Service.AuthorityProducerID,
		MonitorStatus:          monitor,
		AppStatus:              app,
		Targets:                make([]dto.Target, 0, len(view.Targets)),
	}
	for _, target := range view.Targets {
		out.Targets = append(out.Targets, toTarget(target))
	}
	return out, nil
}

func sourceStatuses(states []portService.SourceState) (monitor, app string) {
	monitor, app = "UNKNOWN", "UNKNOWN"
	for _, state := range states {
		switch state.SourceType {
		case "MONITOR":
			monitor = state.Status
		case "APP":
			app = state.Status
		}
	}
	return monitor, app
}

func latestCheckAt(targets []portService.MonitorTarget) *string {
	var latest *time.Time
	for _, target := range targets {
		if target.LastCheckAt == nil {
			continue
		}
		if latest == nil || target.LastCheckAt.After(*latest) {
			latest = target.LastCheckAt
		}
	}
	if latest == nil {
		return nil
	}
	value := latest.UTC().Format(time.RFC3339)
	return &value
}

func toTarget(target portService.MonitorTarget) dto.Target {
	out := dto.Target{
		TargetID: target.TargetID, TargetKey: target.TargetKey, TargetName: target.TargetName,
		TargetType: target.TargetType, Address: target.Address,
		IntervalMS: target.IntervalMS, TimeoutMS: target.TimeoutMS,
		FailureThreshold: target.FailureThreshold, RecoveryThreshold: target.RecoveryThreshold,
		IsRequired: target.IsRequired, Enabled: target.Enabled, Status: target.Status,
		ConsecutiveFailures: target.ConsecutiveFailures, ConsecutiveSuccesses: target.ConsecutiveSuccesses,
		ConfigGeneration: target.ConfigGeneration,
	}
	if len(target.Config) > 0 {
		out.Config = target.Config
	}
	if target.LastCheckAt != nil {
		value := target.LastCheckAt.UTC().Format(time.RFC3339)
		out.LastCheckAt = &value
	}
	return out
}

func toAppTargetInputs(inputs []dto.TargetInput) []appservice.TargetInput {
	out := make([]appservice.TargetInput, 0, len(inputs))
	for _, input := range inputs {
		out = append(out, toAppTargetInput(input))
	}
	return out
}

func toAppTargetInput(input dto.TargetInput) appservice.TargetInput {
	return appservice.TargetInput{
		TargetKey: input.TargetKey, TargetName: input.TargetName, Address: input.Address,
		TargetType: input.TargetType, Config: input.Config,
		IntervalMS: input.IntervalMS, TimeoutMS: input.TimeoutMS,
		FailureThreshold: input.FailureThreshold, RecoveryThreshold: input.RecoveryThreshold,
		IsRequired: input.IsRequired,
	}
}

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func intQuery(c *gin.Context, key string, fallback, max int) int {
	raw := strings.TrimSpace(c.Query(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return fallback
	}
	if value > max {
		return max
	}
	return value
}

func writeErr(c *gin.Context, err error) {
	normalized := apperr.Normalize(err)
	var ae *apperr.Error
	if errors.As(normalized, &ae) {
		c.JSON(apierr.HTTPStatus(ae.Code), gin.H{"code": ae.Code, "message": ae.Message})
		return
	}
	c.JSON(500, gin.H{"code": "INTERNAL", "message": "internal error"})
}

func rawMessageBytes(value *json.RawMessage) []byte {
	if value == nil {
		return nil
	}
	return []byte(*value)
}

func (h *Registry) listEnabledTargets(c *gin.Context) {
	targets, err := h.uc.ListEnabledTargets(c.Request.Context())
	if err != nil {
		writeErr(c, err)
		return
	}
	out := make([]dto.Target, 0, len(targets))
	for _, target := range targets {
		out = append(out, toTarget(target))
	}
	c.JSON(200, out)
}

func (h *Registry) getTarget(c *gin.Context) {
	target, err := h.uc.GetTarget(c.Request.Context(), c.Param("target_id"))
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(200, toTarget(target))
}

func (h *Registry) probeResult(c *gin.Context) {
	var payload dto.ProbeResultPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		writeErr(c, apperr.Validation("invalid json body"))
		return
	}
	lastCheckAt := time.Now().UTC()
	if payload.LastCheckAt != nil && *payload.LastCheckAt != "" {
		if parsed, err := time.Parse(time.RFC3339, *payload.LastCheckAt); err == nil {
			lastCheckAt = parsed
		}
	}
	if err := h.uc.ApplyProbeResult(c.Request.Context(), c.Param("target_id"), payload.Status,
		payload.ConsecutiveFailures, payload.ConsecutiveSuccesses, lastCheckAt); err != nil {
		writeErr(c, err)
		return
	}
	c.Status(204)
}

func (h *Registry) upsertSourceState(c *gin.Context) {
	var payload dto.SourceState
	if err := c.ShouldBindJSON(&payload); err != nil {
		writeErr(c, apperr.Validation("invalid json body"))
		return
	}
	if payload.SourceType == "" || payload.Status == "" {
		writeErr(c, apperr.Validation("source_type and status are required"))
		return
	}
	if err := h.uc.UpsertSourceState(c.Request.Context(), portService.SourceState{
		ServiceID: c.Param("service_id"), SourceType: payload.SourceType,
		Status: payload.Status, Reason: payload.Reason,
	}); err != nil {
		writeErr(c, err)
		return
	}
	c.Status(204)
}
