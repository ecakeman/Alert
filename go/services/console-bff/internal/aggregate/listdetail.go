package aggregate

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/ecakeman/alert/services/platform/httpx"
)

// EpisodeListItem 是路由 11 的列表项（P0 11 的 14 键）。
type EpisodeListItem struct {
	ActiveTaskStatus      string  `json:"active_task_status"`
	ActiveTaskType        string  `json:"active_task_type"`
	ArchivedAt            *string `json:"archived_at"`
	EpisodeID             string  `json:"episode_id"`
	InputVersion          int64   `json:"input_version"`
	ProcessedInputVersion int64   `json:"processed_input_version"`
	ResolutionPending     bool    `json:"resolution_pending"`
	ResolvedAt            *string `json:"resolved_at"`
	ServiceID             string  `json:"service_id"`
	ServiceName           string  `json:"service_name"`
	StartedAt             string  `json:"started_at"`
	StartedBy             string  `json:"started_by"`
	Status                string  `json:"status"`
	UpdatedAt             string  `json:"updated_at"`
}

type episodesListPage struct {
	Total    int               `json:"total"`
	Limit    int               `json:"limit"`
	Offset   int               `json:"offset"`
	Episodes []EpisodeListItem `json:"episodes"`
}

// EpisodeDetailView 是路由 12 的详情（P0 12 的 17 键）。
type EpisodeDetailView struct {
	ActiveTaskID          string  `json:"active_task_id"`
	ActiveTaskStatus      string  `json:"active_task_status"`
	ActiveTaskType        string  `json:"active_task_type"`
	AppStatus             string  `json:"app_status"`
	ArchivedAt            *string `json:"archived_at"`
	EpisodeID             string  `json:"episode_id"`
	FinalInputVersion     int64   `json:"final_input_version"`
	InputHash             string  `json:"input_hash"`
	InputVersion          int64   `json:"input_version"`
	MonitorStatus         string  `json:"monitor_status"`
	ProcessedInputVersion int64   `json:"processed_input_version"`
	ResolutionPending     bool    `json:"resolution_pending"`
	ResolvedAt            *string `json:"resolved_at"`
	ServiceID             string  `json:"service_id"`
	ServiceName           string  `json:"service_name"`
	StartedAt             string  `json:"started_at"`
	StartedBy             string  `json:"started_by"`
	Status                string  `json:"status"`
}

type episodeDetailL2 struct {
	EpisodeID             string  `json:"episode_id"`
	ServiceID             string  `json:"service_id"`
	Status                string  `json:"status"`
	ResolutionPending     bool    `json:"resolution_pending"`
	StartedBy             string  `json:"started_by"`
	StartedAt             string  `json:"started_at"`
	InputHash             string  `json:"input_hash"`
	InputVersion          int64   `json:"input_version"`
	ProcessedInputVersion int64   `json:"processed_input_version"`
	FinalInputVersion     int64   `json:"final_input_version"`
	ResolvedAt            *string `json:"resolved_at"`
	ArchivedAt            *string `json:"archived_at"`
}

type servicesPage struct {
	Total    int               `json:"total"`
	Limit    int               `json:"limit"`
	Offset   int               `json:"offset"`
	Services []ServiceSummaryL1 `json:"services"`
}

func (c Clients) servicesPage(ctx context.Context, query url.Values) (servicesPage, error) {
	var out servicesPage
	err := c.Registry.GetJSON(ctx, "/internal/services?"+query.Encode(), &out)
	if out.Services == nil {
		out.Services = []ServiceSummaryL1{}
	}
	return out, err
}

func (c Clients) serviceViewL1(ctx context.Context, serviceID string) (ServiceViewL1, error) {
	var out ServiceViewL1
	err := c.Registry.GetJSON(ctx, "/internal/services/"+url.PathEscape(serviceID), &out)
	return out, err
}

func (c Clients) episodeDetailL2(ctx context.Context, episodeID string) (episodeDetailL2, error) {
	var out episodeDetailL2
	err := c.Episode.GetJSON(ctx, "/internal/episodes/"+url.PathEscape(episodeID), &out)
	return out, err
}

func (c Clients) activeTaskOne(ctx context.Context, episodeID string) (ActiveTaskL2, bool, error) {
	var out ActiveTaskL2
	err := c.Task.GetJSON(ctx, "/internal/episodes/"+url.PathEscape(episodeID)+"/active-task", &out)
	if err != nil {
		return ActiveTaskL2{}, false, nil
	}
	return out, true, nil
}

func pageQuery(limit, offset int) url.Values {
	q := url.Values{}
	q.Set("limit", strconv.Itoa(limit))
	q.Set("offset", strconv.Itoa(offset))
	return q
}

func (s *Service) servicesList(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), s.timeout)
	defer cancel()
	limit := queryInt(r, "limit", 20)
	offset := queryInt(r, "offset", 0)
	page, err := s.clients.servicesPage(ctx, pageQuery(limit, offset))
	if err != nil {
		s.unavailable(w, "registry", err)
		return
	}
	ids := make([]string, 0, len(page.Services))
	for _, svc := range page.Services {
		ids = append(ids, svc.ServiceID)
	}
	active, err := s.clients.activeByServices(ctx, ids)
	if err != nil {
		s.unavailable(w, "episode", err)
		return
	}
	for i := range page.Services {
		if id, ok := active[page.Services[i].ServiceID]; ok && id != "" {
			v := id
			page.Services[i].ActiveEpisodeID = &v
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"total": page.Total, "limit": page.Limit, "offset": page.Offset, "services": page.Services,
	})
}

func (s *Service) serviceDetail(w http.ResponseWriter, r *http.Request, serviceID string) {
	ctx, cancel := context.WithTimeout(r.Context(), s.timeout)
	defer cancel()
	view, err := s.clients.serviceViewL1(ctx, serviceID)
	if err != nil {
		s.unavailable(w, "registry", err)
		return
	}
	active, err := s.clients.activeByServices(ctx, []string{serviceID})
	if err != nil {
		s.unavailable(w, "episode", err)
		return
	}
	if id, ok := active[serviceID]; ok && id != "" {
		v := id
		view.ActiveEpisodeID = &v
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Service) episodesList(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), s.timeout)
	defer cancel()
	limit := queryInt(r, "limit", 20)
	offset := queryInt(r, "offset", 0)
	query := pageQuery(limit, offset)
	page, err := s.clients.listEpisodes(ctx, query)
	if err != nil {
		s.unavailable(w, "episode", err)
		return
	}
	episodeIDs := make([]string, 0, len(page.Episodes))
	serviceIDs := make([]string, 0, len(page.Episodes))
	for _, e := range page.Episodes {
		episodeIDs = append(episodeIDs, e.EpisodeID)
		serviceIDs = append(serviceIDs, e.ServiceID)
	}
	active, err := s.clients.activeTasks(ctx, episodeIDs)
	if err != nil {
		s.unavailable(w, "task", err)
		return
	}
	names, err := s.clients.servicesByIDs(ctx, serviceIDs)
	if err != nil {
		s.unavailable(w, "registry", err)
		return
	}
	items := make([]EpisodeListItem, 0, len(page.Episodes))
	for _, e := range page.Episodes {
		item := EpisodeListItem{
			EpisodeID: e.EpisodeID, ServiceID: e.ServiceID, ServiceName: names[e.ServiceID],
			Status: e.Status, StartedBy: e.StartedBy, ResolutionPending: e.ResolutionPending,
			InputVersion: e.InputVersion, ProcessedInputVersion: e.ProcessedInputVersion,
			StartedAt: e.StartedAt, UpdatedAt: e.UpdatedAt, ResolvedAt: e.ResolvedAt, ArchivedAt: e.ArchivedAt,
		}
		if task, ok := active[e.EpisodeID]; ok {
			item.ActiveTaskStatus = task.Status
			item.ActiveTaskType = task.TaskType
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, episodesListPage{Total: page.Total, Limit: limit, Offset: offset, Episodes: items})
}

func (s *Service) episodeDetail(w http.ResponseWriter, r *http.Request, episodeID string) {
	ctx, cancel := context.WithTimeout(r.Context(), s.timeout)
	defer cancel()
	detail, err := s.clients.episodeDetailL2(ctx, episodeID)
	if err != nil {
		s.unavailable(w, "episode", err)
		return
	}
	svc, err := s.clients.serviceViewL1(ctx, detail.ServiceID)
	if err != nil {
		s.unavailable(w, "registry", err)
		return
	}
	task, hasTask, err := s.clients.activeTaskOne(ctx, episodeID)
	if err != nil {
		s.unavailable(w, "task", err)
		return
	}
	out := EpisodeDetailView{
		EpisodeID: detail.EpisodeID, ServiceID: detail.ServiceID, Status: detail.Status,
		ResolutionPending: detail.ResolutionPending, StartedBy: detail.StartedBy, StartedAt: detail.StartedAt,
		InputHash: detail.InputHash, InputVersion: detail.InputVersion,
		ProcessedInputVersion: detail.ProcessedInputVersion, FinalInputVersion: detail.FinalInputVersion,
		ResolvedAt: detail.ResolvedAt, ArchivedAt: detail.ArchivedAt,
		ServiceName: svc.Name, MonitorStatus: svc.MonitorStatus, AppStatus: svc.AppStatus,
	}
	if hasTask {
		out.ActiveTaskID = task.TaskID
		out.ActiveTaskStatus = task.Status
		out.ActiveTaskType = task.TaskType
	}
	writeJSON(w, http.StatusOK, out)
}

func queryInt(r *http.Request, key string, fallback int) int {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < 0 {
		return fallback
	}
	return v
}

// 编译期确保未使用导入被引用（json 在别处使用）。
var _ = json.Marshal
var _ = fmt.Sprintf
var _ = httpx.HeaderTraceID