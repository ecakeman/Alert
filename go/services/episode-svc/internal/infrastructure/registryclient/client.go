package registryclient

import (
	"context"
	"errors"
	"time"

	"github.com/ecakeman/alert/services/platform/apierr"
	"github.com/ecakeman/alert/services/platform/httpx"

	"github.com/ecakeman/alert/services/episode-svc/internal/application/apperr"
	portservice "github.com/ecakeman/alert/services/episode-svc/internal/application/port/service"
)

// Reader 是 episode 域对 registry 的只读 HTTP 客户端（一次尝试、统一超时、trace 透传）。
type Reader struct {
	http *httpx.Client
}

func New(baseURL string, timeout time.Duration) *Reader {
	return &Reader{http: httpx.NewClient(baseURL, timeout)}
}

type serviceViewDTO struct {
	ServiceID  string `json:"service_id"`
	ServiceKey string `json:"service_key"`
	Name       string `json:"name"`
	Enabled    bool   `json:"enabled"`
}

type sourceStateDTO struct {
	ServiceID  string `json:"service_id"`
	SourceType string `json:"source_type"`
	Status     string `json:"status"`
	Reason     string `json:"reason"`
}

func (r *Reader) Get(ctx context.Context, serviceID string) (portservice.Service, error) {
	var dto serviceViewDTO
	if err := r.http.GetJSON(ctx, "/internal/services/"+serviceID, &dto); err != nil {
		return portservice.Service{}, mapErr(err)
	}
	return portservice.Service{
		ServiceID: dto.ServiceID, ServiceKey: dto.ServiceKey, Name: dto.Name, Enabled: dto.Enabled,
	}, nil
}

func (r *Reader) ListSourceStates(ctx context.Context, serviceID string) ([]portservice.SourceState, error) {
	var dtos []sourceStateDTO
	if err := r.http.GetJSON(ctx, "/internal/services/"+serviceID+"/source-states", &dtos); err != nil {
		return nil, mapErr(err)
	}
	out := make([]portservice.SourceState, 0, len(dtos))
	for _, dto := range dtos {
		out = append(out, portservice.SourceState{
			ServiceID: dto.ServiceID, SourceType: dto.SourceType, Status: dto.Status, Reason: dto.Reason,
		})
	}
	return out, nil
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
		case apierr.CodeAlreadyExists, apierr.CodeStateConflict:
			return apperr.AlreadyExists(ae.Message)
		case apierr.CodeDependencyUnavailable:
			return apperr.DependencyUnavailable(ae.Message)
		}
		return apperr.New(ae.Code, ae.Message)
	}
	return apperr.DependencyUnavailable("registry unavailable")
}