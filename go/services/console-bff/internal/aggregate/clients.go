package aggregate

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/ecakeman/alert/services/platform/httpx"
)

// RegistryCounts 是 registry /internal/stats 的服务分布（L2）。
type RegistryCounts struct {
	Total    int `json:"total"`
	Enabled  int `json:"enabled"`
	Disabled int `json:"disabled"`
	Normal   int `json:"normal"`
	Abnormal int `json:"abnormal"`
	Unknown  int `json:"unknown"`
}

// RegistryStats 是 registry /internal/stats 的最小投影。
type RegistryStats struct {
	Services RegistryCounts `json:"services"`
	Targets  map[string]int `json:"targets"`
}

// MonitorStats 是 monitor /internal/stats 的最小投影。
type MonitorStats struct {
	Services map[string]int `json:"services"`
	Targets  map[string]int `json:"targets"`
}

// EpisodeStats 是 episode /internal/stats 的最小投影。
type EpisodeStats struct {
	Active   int `json:"active"`
	Archived int `json:"archived"`
	Resolved int `json:"resolved"`
}

// EpisodeSummary 是 /internal/episodes 列表项的最小投影。
type EpisodeSummary struct {
	EpisodeID             string  `json:"episode_id"`
	ServiceID             string  `json:"service_id"`
	Status                string  `json:"status"`
	StartedBy             string  `json:"started_by"`
	ResolutionPending     bool    `json:"resolution_pending"`
	InputVersion          int64   `json:"input_version"`
	ProcessedInputVersion int64   `json:"processed_input_version"`
	StartedAt             string  `json:"started_at"`
	UpdatedAt             string  `json:"updated_at"`
	ResolvedAt            *string `json:"resolved_at"`
	ArchivedAt            *string `json:"archived_at"`
}

// EpisodePage 是 /internal/episodes 的分页投影。
type EpisodePage struct {
	Total    int              `json:"total"`
	Episodes []EpisodeSummary `json:"episodes"`
}

// TimelineParts 是 /internal/episodes/{id}/timeline-parts 的透传投影。
type TimelineParts struct {
	MonitorProjection json.RawMessage `json:"projection"`
	MonitorStartSeq   int64           `json:"monitor_start_seq"`
	AppEvents         json.RawMessage `json:"app_events"`
}

// TaskStats 是 task /internal/stats 的投影。
type TaskStats struct {
	Statuses map[string]int `json:"statuses"`
}

// ActiveTaskL2 是 task /internal/batch/active-tasks 的条目。
type ActiveTaskL2 struct {
	TaskID     string `json:"task_id"`
	TaskType   string `json:"task_type"`
	Status     string `json:"status"`
	Generation int64  `json:"generation"`
}

// ServiceLite 是 registry /internal/services?ids= 的最小条目。
type ServiceLite struct {
	ServiceID string `json:"service_id"`
	Name      string `json:"name"`
}

type servicesPageLite struct {
	Services []ServiceLite `json:"services"`
}

// Clients 聚合四域的内部 HTTP 客户端。
type Clients struct {
	Registry *httpx.Client
	Episode  *httpx.Client
	Monitor  *httpx.Client
	Task     *httpx.Client
}

func (c Clients) registryStats(ctx context.Context) (RegistryStats, error) {
	var out RegistryStats
	err := c.Registry.GetJSON(ctx, "/internal/stats", &out)
	return out, err
}

func (c Clients) monitorStats(ctx context.Context) (MonitorStats, error) {
	var out MonitorStats
	err := c.Monitor.GetJSON(ctx, "/internal/stats", &out)
	return out, err
}

func (c Clients) episodeStats(ctx context.Context) (EpisodeStats, error) {
	var out EpisodeStats
	err := c.Episode.GetJSON(ctx, "/internal/stats", &out)
	return out, err
}

func (c Clients) listEpisodes(ctx context.Context, query url.Values) (EpisodePage, error) {
	var out EpisodePage
	err := c.Episode.GetJSON(ctx, "/internal/episodes?"+query.Encode(), &out)
	return out, err
}

func (c Clients) timelineParts(ctx context.Context, episodeID string, limit int) (TimelineParts, error) {
	var out TimelineParts
	path := fmt.Sprintf("/internal/episodes/%s/timeline-parts?limit=%d", url.PathEscape(episodeID), limit)
	err := c.Episode.GetJSON(ctx, path, &out)
	return out, err
}

func (c Clients) latestEpisodeForService(ctx context.Context, serviceID string) (EpisodeSummary, error) {
	query := url.Values{}
	query.Set("service_id", serviceID)
	query.Set("limit", "1")
	query.Set("offset", "0")
	page, err := c.listEpisodes(ctx, query)
	if err != nil {
		return EpisodeSummary{}, err
	}
	if len(page.Episodes) == 0 {
		return EpisodeSummary{}, errNoEpisode
	}
	return page.Episodes[0], nil
}

func (c Clients) taskStats(ctx context.Context) (TaskStats, error) {
	var out TaskStats
	err := c.Task.GetJSON(ctx, "/internal/stats", &out)
	return out, err
}

func (c Clients) activeTasks(ctx context.Context, episodeIDs []string) (map[string]ActiveTaskL2, error) {
	out := map[string]ActiveTaskL2{}
	if len(episodeIDs) == 0 {
		return out, nil
	}
	if len(episodeIDs) > 200 {
		return nil, fmt.Errorf("active-tasks: episode_ids must contain 1..200 values")
	}
	query := url.Values{}
	query.Set("episode_ids", strings.Join(episodeIDs, ","))
	err := c.Task.GetJSON(ctx, "/internal/batch/active-tasks?"+query.Encode(), &out)
	if out == nil {
		out = map[string]ActiveTaskL2{}
	}
	return out, err
}

func (c Clients) reportFlags(ctx context.Context, episodeIDs []string) (map[string]bool, error) {
	out := map[string]bool{}
	if len(episodeIDs) == 0 {
		return out, nil
	}
	if len(episodeIDs) > 200 {
		return nil, fmt.Errorf("report-flags: episode_ids must contain 1..200 values")
	}
	query := url.Values{}
	query.Set("episode_ids", strings.Join(episodeIDs, ","))
	err := c.Task.GetJSON(ctx, "/internal/batch/report-flags?"+query.Encode(), &out)
	if out == nil {
		out = map[string]bool{}
	}
	return out, err
}

func (c Clients) monitorTrend(ctx context.Context, window time.Duration, buckets int) ([]MonitorTrendPoint, error) {
	query := url.Values{}
	query.Set("window", window.String())
	query.Set("buckets", fmt.Sprintf("%d", buckets))
	var out []MonitorTrendPoint
	err := c.Monitor.GetJSON(ctx, "/internal/points/trend?"+query.Encode(), &out)
	return out, err
}

func (c Clients) servicesByIDs(ctx context.Context, ids []string) (map[string]string, error) {
	out := map[string]string{}
	if len(ids) == 0 {
		return out, nil
	}
	if len(ids) > 200 {
		return nil, fmt.Errorf("services-by-ids: ids must contain 1..200 values")
	}
	query := url.Values{}
	query.Set("ids", strings.Join(ids, ","))
	var page servicesPageLite
	if err := c.Registry.GetJSON(ctx, "/internal/services?"+query.Encode(), &page); err != nil {
		return nil, err
	}
	for _, svc := range page.Services {
		out[svc.ServiceID] = svc.Name
	}
	return out, nil
}

// activeByServices 一次批量补齐 service_id → active episode_id（1..200，空 ids 不发请求）。
func (c Clients) activeByServices(ctx context.Context, ids []string) (map[string]string, error) {
	if len(ids) == 0 {
		return map[string]string{}, nil
	}
	if len(ids) > 200 {
		return nil, fmt.Errorf("active-by-services: ids must contain 1..200 values")
	}
	query := url.Values{}
	query.Set("service_ids", strings.Join(ids, ","))
	var out map[string]string
	if err := c.Episode.GetJSON(ctx, "/internal/episodes/active-by-services?"+query.Encode(), &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = map[string]string{}
	}
	return out, nil
}

var errNoEpisode = fmt.Errorf("no episode for service")

// DefaultTimeout 是聚合总预算（design D：800ms）。
func DefaultTimeout(d time.Duration) time.Duration {
	if d <= 0 {
		return 800 * time.Millisecond
	}
	return d
}