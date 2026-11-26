package episode

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ecakeman/alert/services/contracts"

	"github.com/ecakeman/alert/services/episode-svc/internal/application/apperr"
	portepisode "github.com/ecakeman/alert/services/episode-svc/internal/application/port/episode"
	portshared "github.com/ecakeman/alert/services/episode-svc/internal/application/port/shared"
	porttask "github.com/ecakeman/alert/services/episode-svc/internal/application/port/task"
)

// buildDispatchSnapshot 按 reason 计算输入区间；无新输入或版本非法时返回 false（不写快照）。
func buildDispatchSnapshot(ep portepisode.Episode, reason string, readiness porttask.ArtifactReadiness) (contracts.DispatchSnapshot, bool) {
	to := ep.InputVersion
	from := ep.ProcessedInputVersion + 1
	if reason == "EPISODE_RESOLVED" {
		if to <= 0 {
			return contracts.DispatchSnapshot{}, false
		}
		from = to
	}
	if to <= 0 || from > to {
		return contracts.DispatchSnapshot{}, false
	}
	return contracts.DispatchSnapshot{
		EpisodeID:             ep.EpisodeID,
		ServiceID:             ep.ServiceID,
		EpisodeStatus:         ep.Status,
		InputVersionFrom:      from,
		InputVersionTo:        to,
		MonitorWatermark:      ep.MonitorWatermark,
		AppEventWatermark:     ep.AppEventWatermark,
		FinalInputVersion:     ep.FinalInputVersion,
		ProcessedInputVersion: ep.ProcessedInputVersion,
		Reason:                reason,
		ArtifactReadiness: contracts.ArtifactReadiness{
			Investigation:   readiness.Investigation,
			DiagnosisReport: readiness.DiagnosisReport,
			HashesMatch:     readiness.HashesMatch,
		},
	}, true
}

// appendDispatchable 在同一事务内写入派发决策快照；无新输入或版本非法时不写。
func (s *Service) appendDispatchable(ctx context.Context, tx portshared.Tx, ep portepisode.Episode,
	reason string, readiness porttask.ArtifactReadiness) error {
	snap, ok := buildDispatchSnapshot(ep, reason, readiness)
	if !ok {
		return nil
	}
	body, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	key := fmt.Sprintf("episode.dispatchable:%s:%d:%s", snap.EpisodeID, snap.InputVersionTo, snap.Reason)
	return s.deps.Outbox.Append(ctx, tx, contracts.Event{
		EventID: s.deps.IDs.NewID(), EventType: contracts.EventEpisodeDispatchable, SchemaVersion: 1,
		AggregateType: "episode", AggregateID: snap.EpisodeID,
		OccurredAt: s.deps.Clock.Now().UTC(), Payload: body,
	}, &key)
}

// preDispatchReadiness 在事务前取活动 episode 的产物就绪度；无活动 episode 时全 false。
// 远程查询不得进入事务（design 2.1.3.4）。
func (s *Service) preDispatchReadiness(ctx context.Context, serviceID string) (porttask.ArtifactReadiness, error) {
	var episodeID string
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		active, err := s.deps.Episodes.GetActiveByService(ctx, tx, serviceID)
		if err != nil {
			if isAppCode(err, apperr.CodeNotFound) {
				return nil
			}
			return err
		}
		episodeID = active.EpisodeID
		return nil
	})
	if err != nil {
		return porttask.ArtifactReadiness{}, err
	}
	if episodeID == "" {
		return porttask.ArtifactReadiness{}, nil
	}
	return s.deps.Tasks.GetArtifactReadiness(ctx, episodeID)
}
