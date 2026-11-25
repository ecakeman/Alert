package registryclient

import (
	"context"
	"errors"
	"time"

	"github.com/ecakeman/alert/services/platform/apierr"
	"github.com/ecakeman/alert/services/platform/httpx"

	"github.com/ecakeman/alert/services/system-core/internal/application/apperr"
	portService "github.com/ecakeman/alert/services/system-core/internal/application/port/service"
	portshared "github.com/ecakeman/alert/services/system-core/internal/application/port/shared"
)

// Client 是 registry 域间 HTTP 客户端（P2 过渡：单体只读/命令路径）。
type Client struct {
	http    *httpx.Client
	service serviceRepo
	target  targetRepo
	source  sourceRepo
}

func New(baseURL string, timeout time.Duration) *Client {
	c := &Client{http: httpx.NewClient(baseURL, timeout)}
	c.service.client = c.http
	c.target.client = c.http
	c.source.client = c.http
	return c
}

func (c *Client) ServiceRepo() portService.ServiceRepo         { return c.service }
func (c *Client) TargetRepo() portService.TargetRepo           { return c.target }
func (c *Client) SourceStateRepo() portService.SourceStateRepo { return c.source }

type targetDTO struct {
	TargetID             string  `json:"target_id"`
	TargetKey            string  `json:"target_key"`
	TargetName           string  `json:"target_name"`
	TargetType           string  `json:"target_type"`
	Address              string  `json:"address"`
	IntervalMS           int     `json:"interval_ms"`
	TimeoutMS            int     `json:"timeout_ms"`
	FailureThreshold     int     `json:"failure_threshold"`
	RecoveryThreshold    int     `json:"recovery_threshold"`
	IsRequired           bool    `json:"is_required"`
	Enabled              bool    `json:"enabled"`
	Status               string  `json:"status"`
	ConsecutiveFailures  int     `json:"consecutive_failures"`
	ConsecutiveSuccesses int     `json:"consecutive_successes"`
	LastCheckAt          *string `json:"last_check_at"`
	ConfigGeneration     int64   `json:"config_generation"`
}

type serviceViewDTO struct {
	ServiceID              string      `json:"service_id"`
	ServiceKey             string      `json:"service_key"`
	Name                   string      `json:"name"`
	Description            string      `json:"description"`
	Enabled                bool        `json:"enabled"`
	MonitorAggregationMode string      `json:"monitor_aggregation_mode"`
	AuthorityProducerID    string      `json:"authority_producer_id"`
	MonitorStatus          string      `json:"monitor_status"`
	AppStatus              string      `json:"app_status"`
	Targets                []targetDTO `json:"targets"`
}

type sourceStateDTO struct {
	ServiceID  string `json:"service_id"`
	SourceType string `json:"source_type"`
	Status     string `json:"status"`
	Reason     string `json:"reason"`
}

type probeResultPayload struct {
	Status               string `json:"status,omitempty"`
	ConsecutiveFailures  int    `json:"consecutive_failures"`
	ConsecutiveSuccesses int    `json:"consecutive_successes"`
	LastCheckAt          string `json:"last_check_at"`
}

func toAppTarget(dto targetDTO) portService.MonitorTarget {
	out := portService.MonitorTarget{
		TargetID: dto.TargetID, TargetKey: dto.TargetKey, TargetName: dto.TargetName,
		TargetType: dto.TargetType, Address: dto.Address,
		IntervalMS: dto.IntervalMS, TimeoutMS: dto.TimeoutMS,
		FailureThreshold: dto.FailureThreshold, RecoveryThreshold: dto.RecoveryThreshold,
		IsRequired: dto.IsRequired, Enabled: dto.Enabled, Status: dto.Status,
		ConsecutiveFailures: dto.ConsecutiveFailures, ConsecutiveSuccesses: dto.ConsecutiveSuccesses,
		ConfigGeneration: dto.ConfigGeneration,
	}
	if dto.LastCheckAt != nil {
		if parsed, err := time.Parse(time.RFC3339, *dto.LastCheckAt); err == nil {
			out.LastCheckAt = &parsed
		}
	}
	return out
}

func mapErr(err error) error {
	if err == nil {
		return nil
	}
	var ae *apierr.Error
	if errors.As(err, &ae) {
		switch ae.Code {
		case apierr.CodeNotFound:
			return apperr.NotFound(ae.Message)
		case apierr.CodeValidation:
			return apperr.Validation(ae.Message)
		case apierr.CodeAlreadyExists:
			return apperr.AlreadyExists(ae.Message)
		case apierr.CodeDependencyUnavailable:
			return apperr.DependencyUnavailable(ae.Message)
		case apierr.CodeNotImplemented:
			return apperr.NotImplemented(ae.Message)
		}
		return apperr.New(ae.Code, ae.Message)
	}
	return apperr.DependencyUnavailable("registry unavailable")
}

func notImplemented(method string) error {
	return apperr.NotImplemented("registryclient." + method + " is not supported in P2")
}

// ---------- ServiceRepo ----------

type serviceRepo struct{ client *httpx.Client }

func (r serviceRepo) Get(ctx context.Context, _ portshared.Tx, serviceID string) (portService.Service, error) {
	var dto serviceViewDTO
	if err := r.client.GetJSON(ctx, "/internal/services/"+serviceID, &dto); err != nil {
		return portService.Service{}, mapErr(err)
	}
	return portService.Service{
		ServiceID: dto.ServiceID, ServiceKey: dto.ServiceKey, Name: dto.Name,
		Description: dto.Description, Enabled: dto.Enabled,
		MonitorAggregationMode: dto.MonitorAggregationMode,
		AuthorityProducerID:    dto.AuthorityProducerID,
	}, nil
}

func (r serviceRepo) Create(context.Context, portshared.Tx, portService.Service) error {
	return notImplemented("services.create")
}
func (r serviceRepo) GetByKey(context.Context, portshared.Tx, string) (portService.Service, error) {
	return portService.Service{}, notImplemented("services.get_by_key")
}
func (r serviceRepo) List(context.Context, portshared.Tx, portService.ServiceFilter) ([]portService.Service, error) {
	return nil, notImplemented("services.list")
}
func (r serviceRepo) Count(context.Context, portshared.Tx, portService.ServiceFilter) (int, error) {
	return 0, notImplemented("services.count")
}
func (r serviceRepo) Patch(context.Context, portshared.Tx, string, portService.ServicePatch) error {
	return notImplemented("services.patch")
}
func (r serviceRepo) SoftDelete(context.Context, portshared.Tx, string, time.Time) error {
	return notImplemented("services.soft_delete")
}

// ---------- TargetRepo ----------

type targetRepo struct{ client *httpx.Client }

func (r targetRepo) Get(ctx context.Context, _ portshared.Tx, targetID string) (portService.MonitorTarget, error) {
	var dto targetDTO
	if err := r.client.GetJSON(ctx, "/internal/targets/"+targetID, &dto); err != nil {
		return portService.MonitorTarget{}, mapErr(err)
	}
	return toAppTarget(dto), nil
}

func (r targetRepo) ListByService(ctx context.Context, _ portshared.Tx, serviceID string, _ bool) ([]portService.MonitorTarget, error) {
	var dtos []targetDTO
	if err := r.client.GetJSON(ctx, "/internal/services/"+serviceID+"/targets", &dtos); err != nil {
		return nil, mapErr(err)
	}
	out := make([]portService.MonitorTarget, 0, len(dtos))
	for _, dto := range dtos {
		out = append(out, toAppTarget(dto))
	}
	return out, nil
}

func (r targetRepo) ListEnabled(ctx context.Context, _ portshared.Tx) ([]portService.MonitorTarget, error) {
	var dtos []targetDTO
	if err := r.client.GetJSON(ctx, "/internal/targets", &dtos); err != nil {
		return nil, mapErr(err)
	}
	out := make([]portService.MonitorTarget, 0, len(dtos))
	for _, dto := range dtos {
		out = append(out, toAppTarget(dto))
	}
	return out, nil
}

func (r targetRepo) UpdateProbeResult(ctx context.Context, _ portshared.Tx, targetID, status string, failures, successes int, lastCheckAt time.Time) error {
	return mapErr(r.client.PostJSON(ctx, "/internal/targets/"+targetID+"/probe-result", probeResultPayload{
		Status: status, ConsecutiveFailures: failures, ConsecutiveSuccesses: successes,
		LastCheckAt: lastCheckAt.UTC().Format(time.RFC3339),
	}, nil))
}

func (r targetRepo) UpdateLastCheckAt(ctx context.Context, _ portshared.Tx, targetID string, at time.Time) error {
	return mapErr(r.client.PostJSON(ctx, "/internal/targets/"+targetID+"/probe-result", probeResultPayload{
		LastCheckAt: at.UTC().Format(time.RFC3339),
	}, nil))
}

func (r targetRepo) Create(context.Context, portshared.Tx, portService.MonitorTarget) error {
	return notImplemented("targets.create")
}
func (r targetRepo) Patch(context.Context, portshared.Tx, string, portService.TargetPatch) error {
	return notImplemented("targets.patch")
}
func (r targetRepo) SoftDelete(context.Context, portshared.Tx, string, time.Time) error {
	return notImplemented("targets.soft_delete")
}

// ---------- SourceStateRepo ----------

type sourceRepo struct{ client *httpx.Client }

func (r sourceRepo) ListByService(ctx context.Context, _ portshared.Tx, serviceID string) ([]portService.SourceState, error) {
	var dtos []sourceStateDTO
	if err := r.client.GetJSON(ctx, "/internal/services/"+serviceID+"/source-states", &dtos); err != nil {
		return nil, mapErr(err)
	}
	out := make([]portService.SourceState, 0, len(dtos))
	for _, dto := range dtos {
		out = append(out, portService.SourceState{
			ServiceID: dto.ServiceID, SourceType: dto.SourceType, Status: dto.Status, Reason: dto.Reason,
		})
	}
	return out, nil
}

func (r sourceRepo) Get(ctx context.Context, tx portshared.Tx, serviceID, sourceType string) (portService.SourceState, error) {
	states, err := r.ListByService(ctx, tx, serviceID)
	if err != nil {
		return portService.SourceState{}, err
	}
	for _, state := range states {
		if state.SourceType == sourceType {
			return state, nil
		}
	}
	return portService.SourceState{}, apperr.NotFound("source state not found")
}

func (r sourceRepo) Upsert(ctx context.Context, _ portshared.Tx, state portService.SourceState) error {
	return mapErr(r.client.PostJSON(ctx, "/internal/services/"+state.ServiceID+"/source-states", sourceStateDTO{
		ServiceID: state.ServiceID, SourceType: state.SourceType, Status: state.Status, Reason: state.Reason,
	}, nil))
}
