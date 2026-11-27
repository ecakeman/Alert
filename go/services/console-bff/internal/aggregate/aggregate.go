package aggregate

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ecakeman/alert/services/platform/apierr"
)

// Service 是 P5.10 的 BFF 聚合入口：dashboard（路由 1）与两条 timeline（14/15）。
type Service struct {
	clients Clients
	timeout time.Duration
	logger  *slog.Logger
}

func New(clients Clients, timeout time.Duration, logger *slog.Logger) *Service {
	return &Service{clients: clients, timeout: DefaultTimeout(timeout), logger: logger}
}

// L1Route 标识路由在 L1 出口需要的投影类型（D-24）。
type L1Route int

const (
	L1None L1Route = iota
	L1ServiceViewNoActive
	L1ServiceViewActive
	L1TargetsList
	L1Target
)

// L1Projector 由 aggregate.Service 实现，供 proxy 的 ModifyResponse 调用。
type L1Projector interface {
	ProjectL1(ctx context.Context, route L1Route, upstream []byte) ([]byte, error)
}

// ProjectL1 是 L1 投影的唯一分派点：先解析视图，再按需一次批量补 active，最后编码。
func (s *Service) ProjectL1(ctx context.Context, route L1Route, upstream []byte) ([]byte, error) {
	switch route {
	case L1None:
		return upstream, nil
	case L1Target:
		return projectTargetL1(upstream)
	case L1TargetsList:
		return projectTargetsListL1(upstream)
	case L1ServiceViewNoActive:
		return projectServiceViewL1(upstream, nil)
	case L1ServiceViewActive:
		var view ServiceViewL1
		if err := json.Unmarshal(upstream, &view); err != nil {
			return nil, fmt.Errorf("decode service view for active: %w", err)
		}
		active := map[string]string{}
		if view.ServiceID != "" {
			got, err := s.clients.activeByServices(ctx, []string{view.ServiceID})
			if err != nil {
				return nil, fmt.Errorf("active episode lookup: %w", err)
			}
			active = got
		}
		return projectServiceViewL1(upstream, active)
	default:
		return upstream, nil
	}
}

func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		apierr.Write(w, apierr.New("METHOD_NOT_ALLOWED", "method not allowed"))
		return
	}
	segments := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	// /api/v1/dashboard/summary
	if len(segments) == 4 && segments[0] == "api" && segments[1] == "v1" &&
		segments[2] == "dashboard" && segments[3] == "summary" {
		s.dashboard(w, r)
		return
	}
	// /api/v1/services/{id}/timeline 或 /api/v1/episodes/{id}/timeline
	if len(segments) == 5 && segments[0] == "api" && segments[1] == "v1" && segments[4] == "timeline" {
		switch segments[2] {
		case "services":
			s.serviceTimeline(w, r, segments[3])
			return
		case "episodes":
			s.episodeTimeline(w, r, segments[3])
			return
		}
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"code": "NOT_FOUND", "message": "resource not found"})
}

func (s *Service) dashboard(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), s.timeout)
	defer cancel()

	stats, err := s.clients.registryStats(ctx)
	if err != nil {
		s.unavailable(w, "registry", err)
		return
	}
	episodeCounts, err := s.clients.episodeStats(ctx)
	if err != nil {
		s.unavailable(w, "episode", err)
		return
	}
	page, err := s.clients.listEpisodes(ctx, queryLimit(5))
	if err != nil {
		s.unavailable(w, "episode", err)
		return
	}

	recent := make([]RecentEpisode, 0, len(page.Episodes))
	for _, e := range page.Episodes {
		recent = append(recent, RecentEpisode{
			EpisodeID:             e.EpisodeID,
			ServiceID:             e.ServiceID,
			Status:                e.Status,
			StartedBy:             e.StartedBy,
			InputVersion:          e.InputVersion,
			ProcessedInputVersion: e.ProcessedInputVersion,
			StartedAt:             e.StartedAt,
			UpdatedAt:             e.UpdatedAt,
		})
	}

	out := Dashboard{
		Episodes: EpisodeCounts{
			Active: episodeCounts.Active, Archived: episodeCounts.Archived, Resolved: episodeCounts.Resolved,
		},
		GeneratedAt:    time.Now().UTC().Format(time.RFC3339Nano),
		MonitorTrend:   []TrendPoint{},
		RecentEpisodes: recent,
		Services: ServiceCounts{
			Total: stats.Services,
			// P6.2 由 registry stats 的分布补齐；P5.10 保持零值，形状不变。
		},
		Targets: targetCounts(stats.Targets),
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Service) serviceTimeline(w http.ResponseWriter, r *http.Request, serviceID string) {
	ctx, cancel := context.WithTimeout(r.Context(), s.timeout)
	defer cancel()
	episode, err := s.clients.latestEpisodeForService(ctx, serviceID)
	if err != nil {
		s.unavailable(w, "episode", err)
		return
	}
	s.writeTimeline(w, ctx, episode.EpisodeID)
}

func (s *Service) episodeTimeline(w http.ResponseWriter, r *http.Request, episodeID string) {
	ctx, cancel := context.WithTimeout(r.Context(), s.timeout)
	defer cancel()
	s.writeTimeline(w, ctx, episodeID)
}

func (s *Service) writeTimeline(w http.ResponseWriter, ctx context.Context, episodeID string) {
	parts, err := s.clients.timelineParts(ctx, episodeID, 200)
	if err != nil {
		s.unavailable(w, "episode", err)
		return
	}
	writeJSON(w, http.StatusOK, parts)
}

func (s *Service) unavailable(w http.ResponseWriter, upstream string, err error) {
	if s.logger != nil {
		s.logger.Warn("aggregate upstream failed", "upstream", upstream, "reason", err.Error())
	}
	apierr.Write(w, apierr.DependencyUnavailable("upstream unavailable"))
}

func targetCounts(targets map[string]int) TargetCounts {
	var out TargetCounts
	for status, count := range targets {
		switch strings.ToLower(status) {
		case "normal":
			out.Normal += count
		case "abnormal":
			out.Abnormal += count
		case "unknown":
			out.Unknown += count
		}
		out.Total += count
	}
	return out
}

func queryLimit(limit int) url.Values {
	q := url.Values{}
	q.Set("limit", strconv.Itoa(limit))
	q.Set("offset", "0")
	return q
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}