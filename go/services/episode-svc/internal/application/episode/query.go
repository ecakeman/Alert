package episode

import (
	"context"
	"time"

	"github.com/ecakeman/alert/services/contracts"

	"github.com/ecakeman/alert/services/episode-svc/internal/application/apperr"
	portepisode "github.com/ecakeman/alert/services/episode-svc/internal/application/port/episode"
	portshared "github.com/ecakeman/alert/services/episode-svc/internal/application/port/shared"
)

// EpisodePage 是列表查询结果（分页元信息 + 条目）。
type EpisodePage struct {
	Total    int
	Limit    int
	Offset   int
	Episodes []portepisode.Episode
}

// ListEpisodes 返回 episode 列表；跨域展示字段由 BFF 补齐，域内只返回自身事实。
func (s *Service) ListEpisodes(ctx context.Context, f portepisode.EpisodeFilter) (EpisodePage, error) {
	if f.Limit <= 0 {
		f.Limit = 20
	}
	if f.Limit > 200 {
		f.Limit = 200
	}
	page := EpisodePage{Limit: f.Limit, Offset: f.Offset}
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		total, err := s.deps.Episodes.Count(ctx, tx, f)
		if err != nil {
			return err
		}
		list, err := s.deps.Episodes.List(ctx, tx, f)
		if err != nil {
			return err
		}
		page.Total = total
		page.Episodes = list
		return nil
	})
	return page, err
}

// GetEpisode 返回单条 episode 事实。
func (s *Service) GetEpisode(ctx context.Context, episodeID string) (portepisode.Episode, error) {
	var ep portepisode.Episode
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		var err error
		ep, err = s.deps.Episodes.Get(ctx, tx, episodeID)
		return err
	})
	return ep, err
}

// ActiveByServices 返回 service_id → ACTIVE episode_id 的批量映射（缺失即无 ACTIVE）。
func (s *Service) ActiveByServices(ctx context.Context, serviceIDs []string) (map[string]string, error) {
	out := make(map[string]string, len(serviceIDs))
	if len(serviceIDs) == 0 {
		return out, nil
	}
	if len(serviceIDs) > 200 {
		return nil, apperr.Validation("service_ids must contain 1..200 values")
	}
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		list, err := s.deps.Episodes.ListActiveByServices(ctx, tx, serviceIDs)
		if err != nil {
			return err
		}
		for _, ep := range list {
			out[ep.ServiceID] = ep.EpisodeID
		}
		return nil
	})
	return out, err
}

// PendingDispatch 是 task 对账兜底：返回“有未处理输入且无活动任务”的派发快照。
// 逐条远程查询仅用于低频对账，上限 64（design 2.1.3.5-（5））。
func (s *Service) PendingDispatch(ctx context.Context, limit int) ([]contracts.DispatchSnapshot, error) {
	if limit <= 0 || limit > 64 {
		limit = 64
	}
	var candidates []portepisode.Episode
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		list, err := s.deps.Episodes.List(ctx, tx, portepisode.EpisodeFilter{Status: "ACTIVE", Limit: 200})
		if err != nil {
			return err
		}
		for _, ep := range list {
			if ep.InputVersion > ep.ProcessedInputVersion {
				candidates = append(candidates, ep)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	out := make([]contracts.DispatchSnapshot, 0, limit)
	for _, ep := range candidates {
		if len(out) >= limit {
			break
		}
		active, err := s.deps.Tasks.GetActive(ctx, ep.EpisodeID)
		if err != nil {
			return nil, err
		}
		if active != nil {
			continue
		}
		readiness, err := s.deps.Tasks.GetArtifactReadiness(ctx, ep.EpisodeID)
		if err != nil {
			return nil, err
		}
		if snap, ok := buildDispatchSnapshot(ep, "INPUT_APPENDED", readiness); ok {
			out = append(out, snap)
		}
	}
	return out, nil
}

// ServiceAppEvents 返回服务级 APP 事件（时间轴用）。
func (s *Service) ServiceAppEvents(ctx context.Context, serviceID string, limit int) ([]portepisode.AppEvent, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var out []portepisode.AppEvent
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		var err error
		out, err = s.deps.AppEvents.ListByBound(ctx, tx, serviceID, 0, int64(1)<<62, limit)
		return err
	})
	return out, err
}

// TimelineParts 返回故障时间轴的 episode 域片段（monitor 部分由 BFF 另取）。
type TimelineParts struct {
	Episode        portepisode.Episode
	InputVersion   portepisode.InputVersion
	AppEvents      []portepisode.AppEvent
	MonitorStartSeq int64
}

func (s *Service) TimelineParts(ctx context.Context, episodeID string, limit int) (TimelineParts, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	var parts TimelineParts
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		ep, err := s.deps.Episodes.Get(ctx, tx, episodeID)
		if err != nil {
			return err
		}
		parts.Episode = ep
		iv, err := s.deps.InputVersions.GetLatest(ctx, tx, episodeID)
		if err != nil {
			if !isAppCode(err, apperr.CodeNotFound) {
				return err
			}
		} else {
			parts.InputVersion = iv
			parts.MonitorStartSeq = iv.MonitorWatermark
		}
		events, err := s.deps.AppEvents.ListByBound(ctx, tx, ep.ServiceID, 0, int64(1)<<62, limit)
		if err != nil {
			return err
		}
		for _, e := range events {
			if e.EpisodeID != nil && *e.EpisodeID == episodeID {
				parts.AppEvents = append(parts.AppEvents, e)
			}
		}
		return nil
	})
	return parts, err
}

// Stats 返回 episode 状态计数（dashboard 口径）。
type EpisodeStats struct {
	Active   int `json:"active"`
	Resolved int `json:"resolved"`
	Archived int `json:"archived"`
	Total    int `json:"total"`
}

func (s *Service) Stats(ctx context.Context) (EpisodeStats, error) {
	var st EpisodeStats
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		active, err := s.deps.Episodes.Count(ctx, tx, portepisode.EpisodeFilter{Status: "ACTIVE"})
		if err != nil {
			return err
		}
		resolved, err := s.deps.Episodes.Count(ctx, tx, portepisode.EpisodeFilter{Status: "RESOLVED"})
		if err != nil {
			return err
		}
		archived, err := s.deps.Episodes.Count(ctx, tx, portepisode.EpisodeFilter{Status: "ARCHIVED"})
		if err != nil {
			return err
		}
		st = EpisodeStats{Active: active, Resolved: resolved, Archived: archived, Total: active + resolved + archived}
		return nil
	})
	return st, err
}

var _ = time.Now