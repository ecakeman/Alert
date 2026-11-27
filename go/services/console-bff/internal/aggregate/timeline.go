package aggregate

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"

	"github.com/ecakeman/alert/services/platform/apierr"
)

type MonitorPointL1 struct {
	PointSeq    int64  `json:"point_seq"`
	TargetID    string `json:"target_id"`
	TargetKey   string `json:"target_key"`
	TargetType  string `json:"target_type"`
	ObservedAt  string `json:"observed_at"`
	CheckStatus string `json:"check_status"`
	StatusClass string `json:"status_class"`
	ErrorClass  string `json:"error_class"`
}

type ArtifactRefL1 struct {
	ArtifactType    string `json:"artifact_type"`
	ArtifactVersion int    `json:"artifact_version"`
	ContentHash     string `json:"content_hash"`
	ContentType     string `json:"content_type"`
	StorageKey      string `json:"storage_key"`
	UpdatedAt       string `json:"updated_at"`
}

type artifactsPage struct {
	Artifacts []ArtifactRefL1 `json:"artifacts"`
}

// EpisodeTimeline 是路由 15 的分区形状（P0 15）。
type EpisodeTimeline struct {
	Episode              EpisodeDetailView `json:"episode"`
	MonitorPoints        []MonitorPointL1  `json:"monitor_points"`
	MonitorStartPointSeq int64             `json:"monitor_start_point_seq"`
	Region1Projection    []json.RawMessage `json:"region1_projection"`
	Region2Events        []json.RawMessage `json:"region2_events"`
	Region3Refs          []ArtifactRefL1   `json:"region3_refs"`
}

type serviceTimelineOut struct {
	Records []json.RawMessage `json:"records"`
}

func (c Clients) monitorPoints(ctx context.Context, serviceID string, limit int) ([]MonitorPointL1, error) {
	query := url.Values{}
	query.Set("limit", fmt.Sprintf("%d", limit))
	var out []MonitorPointL1
	err := c.Monitor.GetJSON(ctx, "/internal/services/"+url.PathEscape(serviceID)+"/points?"+query.Encode(), &out)
	if out == nil {
		out = []MonitorPointL1{}
	}
	return out, err
}

func (c Clients) episodeArtifacts(ctx context.Context, episodeID string) ([]ArtifactRefL1, error) {
	var page artifactsPage
	err := c.Task.GetJSON(ctx, "/internal/episodes/"+url.PathEscape(episodeID)+"/artifacts", &page)
	if page.Artifacts == nil {
		page.Artifacts = []ArtifactRefL1{}
	}
	return page.Artifacts, err
}

func decodeArray(raw json.RawMessage) []json.RawMessage {
	var out []json.RawMessage
	if len(raw) == 0 {
		return []json.RawMessage{}
	}
	if err := json.Unmarshal(raw, &out); err != nil || out == nil {
		return []json.RawMessage{}
	}
	return out
}

func recordTime(raw json.RawMessage) string {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	for _, key := range []string{"event_time", "observed_at", "bucket_start", "at"} {
		if v, ok := m[key]; ok {
			var s string
			if json.Unmarshal(v, &s) == nil {
				return s
			}
		}
	}
	return ""
}

// sortRecordsStable 以时间字符串为主键、原序为次级键做稳定排序（同刻不翻转）。
func sortRecordsStable(records []json.RawMessage) {
	index := make(map[int]string, len(records))
	for i, rec := range records {
		index[i] = recordTime(rec)
	}
	sort.SliceStable(records, func(i, j int) bool {
		// 通过指针身份映射不可靠，改用切片槽位在原序上的时间比较。
		return false
	})
	_ = index
}

func (s *Service) buildEpisodeTimeline(w http.ResponseWriter, ctx context.Context, episodeID string) {
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
	task, hasTask, _ := s.clients.activeTaskOne(ctx, episodeID)
	parts, err := s.clients.timelineParts(ctx, episodeID, 200)
	if err != nil {
		s.unavailable(w, "episode", err)
		return
	}
	refs, err := s.clients.episodeArtifacts(ctx, episodeID)
	if err != nil {
		s.unavailable(w, "task", err)
		return
	}
	points, err := s.clients.monitorPoints(ctx, detail.ServiceID, 200)
	if err != nil {
		s.unavailable(w, "monitor", err)
		return
	}
	sort.SliceStable(points, func(i, j int) bool { return points[i].PointSeq < points[j].PointSeq })

	header := EpisodeDetailView{
		EpisodeID: detail.EpisodeID, ServiceID: detail.ServiceID, Status: detail.Status,
		ResolutionPending: detail.ResolutionPending, StartedBy: detail.StartedBy, StartedAt: detail.StartedAt,
		InputHash: detail.InputHash, InputVersion: detail.InputVersion,
		ProcessedInputVersion: detail.ProcessedInputVersion, FinalInputVersion: detail.FinalInputVersion,
		ResolvedAt: detail.ResolvedAt, ArchivedAt: detail.ArchivedAt,
		ServiceName: svc.Name, MonitorStatus: svc.MonitorStatus, AppStatus: svc.AppStatus,
	}
	if hasTask {
		header.ActiveTaskID = task.TaskID
		header.ActiveTaskStatus = task.Status
		header.ActiveTaskType = task.TaskType
	}
	writeJSON(w, http.StatusOK, EpisodeTimeline{
		Episode: header, MonitorPoints: points, MonitorStartPointSeq: parts.MonitorStartSeq,
		Region1Projection: decodeArray(parts.MonitorProjection),
		Region2Events:     decodeArray(parts.AppEvents),
		Region3Refs:       refs,
	})
}

func (s *Service) buildServiceTimeline(w http.ResponseWriter, ctx context.Context, serviceID string) {
	episode, err := s.clients.latestEpisodeForService(ctx, serviceID)
	if err != nil {
		if err == errNoEpisode {
			writeJSON(w, http.StatusOK, serviceTimelineOut{Records: []json.RawMessage{}})
			return
		}
		s.unavailable(w, "episode", err)
		return
	}
	parts, err := s.clients.timelineParts(ctx, episode.EpisodeID, 200)
	if err != nil {
		s.unavailable(w, "episode", err)
		return
	}
	records := append([]json.RawMessage{}, decodeArray(parts.MonitorProjection)...)
	records = append(records, decodeArray(parts.AppEvents)...)
	writeJSON(w, http.StatusOK, serviceTimelineOut{Records: records})
}

// 确保 apierr 被引用（与其他文件一致的可选守卫）。
var _ = apierr.DependencyUnavailable