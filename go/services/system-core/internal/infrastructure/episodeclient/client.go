package episodeclient

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/ecakeman/alert/services/platform/apierr"
	"github.com/ecakeman/alert/services/platform/httpx"

	"github.com/ecakeman/alert/services/system-core/internal/application/apperr"
	portdashboard "github.com/ecakeman/alert/services/system-core/internal/application/port/dashboard"
	portepisode "github.com/ecakeman/alert/services/system-core/internal/application/port/episode"
	portshared "github.com/ecakeman/alert/services/system-core/internal/application/port/shared"
)

// Client 是 episode-svc 的域间 HTTP 客户端（P4.8：单体 task 侧零直读）。
type Client struct {
	http *httpx.Client
}

func New(baseURL string, timeout time.Duration) *Client {
	return &Client{http: httpx.NewClient(baseURL, timeout)}
}

// EpisodeRepo 返回只读实现；写方法在 P4.8 过渡期显式 NotImplemented。
func (c *Client) EpisodeRepo() portepisode.EpisodeRepo { return repo{http: c.http} }

type episodeDTO struct {
	EpisodeID             string  `json:"episode_id"`
	ServiceID             string  `json:"service_id"`
	Status                string  `json:"status"`
	StartedBy             string  `json:"started_by"`
	ResolutionPending     bool    `json:"resolution_pending"`
	InputHash             string  `json:"input_hash"`
	InputVersion          int64   `json:"input_version"`
	ProcessedInputVersion int64   `json:"processed_input_version"`
	FinalInputVersion     int64   `json:"final_input_version"`
	StartedAt             string  `json:"started_at"`
	ResolvedAt            *string `json:"resolved_at"`
	ArchivedAt            *string `json:"archived_at"`
	UpdatedAt             *string `json:"updated_at"`
}

type pageDTO struct {
	Total    int          `json:"total"`
	Limit    int          `json:"limit"`
	Offset   int          `json:"offset"`
	Episodes []episodeDTO `json:"episodes"`
}

type repo struct{ http *httpx.Client }

func (r repo) Get(ctx context.Context, _ portshared.Tx, episodeID string) (portepisode.Episode, error) {
	var dto episodeDTO
	if err := r.http.GetJSON(ctx, "/internal/episodes/"+episodeID, &dto); err != nil {
		return portepisode.Episode{}, mapErr(err)
	}
	return toApp(dto), nil
}

func (r repo) List(ctx context.Context, _ portshared.Tx, f portepisode.EpisodeFilter) ([]portepisode.Episode, error) {
	q := url.Values{}
	if f.ServiceID != "" {
		q.Set("service_id", f.ServiceID)
	}
	if f.Status != "" {
		q.Set("status", f.Status)
	}
	if f.Limit > 0 {
		q.Set("limit", fmt.Sprint(f.Limit))
	}
	if f.Offset > 0 {
		q.Set("offset", fmt.Sprint(f.Offset))
	}
	var dto pageDTO
	if err := r.http.GetJSON(ctx, "/internal/episodes?"+q.Encode(), &dto); err != nil {
		return nil, mapErr(err)
	}
	out := make([]portepisode.Episode, 0, len(dto.Episodes))
	for _, e := range dto.Episodes {
		out = append(out, toApp(e))
	}
	return out, nil
}

func (r repo) Count(ctx context.Context, _ portshared.Tx, f portepisode.EpisodeFilter) (int, error) {
	q := url.Values{}
	if f.ServiceID != "" {
		q.Set("service_id", f.ServiceID)
	}
	if f.Status != "" {
		q.Set("status", f.Status)
	}
	q.Set("limit", "1")
	var dto pageDTO
	if err := r.http.GetJSON(ctx, "/internal/episodes?"+q.Encode(), &dto); err != nil {
		return 0, mapErr(err)
	}
	return dto.Total, nil
}

func (r repo) GetActiveByService(ctx context.Context, tx portshared.Tx, serviceID string) (portepisode.Episode, error) {
	var mapping map[string]string
	if err := r.http.GetJSON(ctx, "/internal/episodes/active-by-services?service_ids="+url.QueryEscape(serviceID), &mapping); err != nil {
		return portepisode.Episode{}, mapErr(err)
	}
	id := mapping[serviceID]
	if id == "" {
		return portepisode.Episode{}, apperr.NotFound("active episode not found")
	}
	return r.Get(ctx, tx, id)
}

func (r repo) ListActiveByServices(ctx context.Context, tx portshared.Tx, serviceIDs []string) ([]portepisode.Episode, error) {
	raw := ""
	for i, id := range serviceIDs {
		if i > 0 {
			raw += ","
		}
		raw += id
	}
	var mapping map[string]string
	if err := r.http.GetJSON(ctx, "/internal/episodes/active-by-services?service_ids="+url.QueryEscape(raw), &mapping); err != nil {
		return nil, mapErr(err)
	}
	out := make([]portepisode.Episode, 0, len(mapping))
	for _, id := range mapping {
		ep, err := r.Get(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, ep)
	}
	return out, nil
}

func (r repo) Create(context.Context, portshared.Tx, portepisode.Episode) error {
	return apperr.NotImplemented("episode repo: create moved to episode-svc")
}

func (r repo) UpdateStatus(context.Context, portshared.Tx, string, string, time.Time, *time.Time, *time.Time) error {
	return apperr.NotImplemented("episode repo: update moved to episode-svc")
}

func (r repo) ResolveIfUnchanged(context.Context, portshared.Tx, string, time.Time, time.Time) (bool, error) {
	return false, apperr.NotImplemented("episode repo: resolve moved to episode-svc")
}

func (r repo) TouchInputVersion(context.Context, portshared.Tx, string, int64, string, int64, int64) error {
	return apperr.NotImplemented("episode repo: input version moved to episode-svc")
}

func toApp(dto episodeDTO) portepisode.Episode {
	ep := portepisode.Episode{
		EpisodeID: dto.EpisodeID, ServiceID: dto.ServiceID, Status: dto.Status,
		StartedBy: dto.StartedBy, ResolutionPending: dto.ResolutionPending,
		InputHash: dto.InputHash, InputVersion: dto.InputVersion,
		ProcessedInputVersion: dto.ProcessedInputVersion, FinalInputVersion: dto.FinalInputVersion,
	}
	if t, err := time.Parse(time.RFC3339Nano, dto.StartedAt); err == nil {
		ep.StartedAt = t
	}
	if dto.ResolvedAt != nil {
		if t, err := time.Parse(time.RFC3339Nano, *dto.ResolvedAt); err == nil {
			ep.ResolvedAt = &t
		}
	}
	if dto.ArchivedAt != nil {
		if t, err := time.Parse(time.RFC3339Nano, *dto.ArchivedAt); err == nil {
			ep.ArchivedAt = &t
		}
	}
	if dto.UpdatedAt != nil {
		if t, err := time.Parse(time.RFC3339Nano, *dto.UpdatedAt); err == nil {
			ep.UpdatedAt = t
		}
	}
	return ep
}

type statsDTO struct {
	Active   int `json:"active"`
	Resolved int `json:"resolved"`
	Archived int `json:"archived"`
}

// Counts 实现 dashboard 的 episode 计数源。
func (c *Client) Counts(ctx context.Context) (map[string]int, error) {
	var dto statsDTO
	if err := c.http.GetJSON(ctx, "/internal/stats", &dto); err != nil {
		return nil, mapErr(err)
	}
	return map[string]int{"active": dto.Active, "resolved": dto.Resolved, "archived": dto.Archived}, nil
}

// Recent 实现 dashboard 的最近故障源（跨域展示字段由单体 EnrichRecent 补齐）。
func (c *Client) Recent(ctx context.Context, limit int) ([]portdashboard.RecentEpisode, error) {
	var dto pageDTO
	if err := c.http.GetJSON(ctx, fmt.Sprintf("/internal/episodes?limit=%d", limit), &dto); err != nil {
		return nil, mapErr(err)
	}
	out := make([]portdashboard.RecentEpisode, 0, len(dto.Episodes))
	for _, e := range dto.Episodes {
		ep := toApp(e)
		out = append(out, portdashboard.RecentEpisode{
			EpisodeID: ep.EpisodeID, ServiceID: ep.ServiceID, Status: ep.Status,
			ResolutionPending: ep.ResolutionPending, StartedBy: ep.StartedBy,
			StartedAt: ep.StartedAt, ResolvedAt: ep.ResolvedAt, ArchivedAt: ep.ArchivedAt,
			InputVersion: ep.InputVersion, ProcessedInputVersion: ep.ProcessedInputVersion,
			UpdatedAt: ep.UpdatedAt,
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
		case apierr.CodeDependencyUnavailable:
			return apperr.DependencyUnavailable(ae.Message)
		}
		return apperr.New(ae.Code, ae.Message)
	}
	return apperr.DependencyUnavailable("episode unavailable")
}