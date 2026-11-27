package task

import (
	"context"

	"github.com/ecakeman/alert/services/contracts"

	"github.com/ecakeman/alert/services/task-orchestrator/internal/application/apperr"
	portshared "github.com/ecakeman/alert/services/task-orchestrator/internal/application/port/shared"
	"github.com/ecakeman/alert/services/task-orchestrator/internal/domain"
)

// ActiveTaskFor 是 episode 解除判定所需的“活动任务”只读查询；无则 NOT_FOUND。
func (s *Service) ActiveTaskFor(ctx context.Context, episodeID string) (TaskView, error) {
	var view TaskView
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		list, err := s.deps.Tasks.ListActiveByEpisode(ctx, tx, episodeID)
		if err != nil {
			return err
		}
		if len(list) == 0 {
			return apperr.NotFound("active task not found")
		}
		view = toTaskView(list[0])
		return nil
	})
	return view, err
}

// ArtifactReadiness 是 episode 生成 dispatchable 快照所需的产物就绪度。
func (s *Service) ArtifactReadiness(ctx context.Context, episodeID string) (contracts.ArtifactReadiness, error) {
	var out contracts.ArtifactReadiness
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		inv, invErr := s.deps.Artifacts.GetLatest(ctx, tx, episodeID, string(domain.ArtifactInvestigation))
		if invErr != nil && !artifactMissing(invErr) {
			return invErr
		}
		rep, repErr := s.deps.Artifacts.GetLatest(ctx, tx, episodeID, string(domain.ArtifactDiagnosisReport))
		if repErr != nil && !artifactMissing(repErr) {
			return repErr
		}
		hasInv := invErr == nil
		hasRep := repErr == nil
		out = contracts.ArtifactReadiness{
			Investigation:   hasInv,
			DiagnosisReport: hasRep,
			HashesMatch:     hasInv && hasRep && inv.ContentHash != "" && rep.ContentHash != "",
		}
		return nil
	})
	return out, err
}

func artifactMissing(err error) bool {
	code := apperr.Code(err)
	return code == apperr.CodeNotFound || code == apperr.CodeArtifactNotFound
}
// Stats 返回任务按状态计数（dashboard 用）。
func (s *Service) Stats(ctx context.Context) (map[string]int, error) {
	var out map[string]int
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		var err error
		out, err = s.deps.Tasks.CountByStatus(ctx, tx)
		return err
	})
	return out, err
}

// ActiveTasksByEpisodes 批量返回每 episode 的活动任务（1..200；缺失无键）。
func (s *Service) ActiveTasksByEpisodes(ctx context.Context, episodeIDs []string) (map[string]TaskView, error) {
	out := make(map[string]TaskView, len(episodeIDs))
	if len(episodeIDs) == 0 {
		return out, nil
	}
	if len(episodeIDs) > 200 {
		return nil, apperr.Validation("episode_ids must contain 1..200 values")
	}
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		list, err := s.deps.Tasks.ListActiveByEpisodes(ctx, tx, episodeIDs)
		if err != nil {
			return err
		}
		for _, t := range list {
			out[t.EpisodeID] = toTaskView(t)
		}
		return nil
	})
	return out, err
}

// ReportFlagsByEpisodes 批量返回每 episode 是否有诊断报告（1..200；缺失无键）。
func (s *Service) ReportFlagsByEpisodes(ctx context.Context, episodeIDs []string) (map[string]bool, error) {
	out := make(map[string]bool, len(episodeIDs))
	if len(episodeIDs) == 0 {
		return out, nil
	}
	if len(episodeIDs) > 200 {
		return nil, apperr.Validation("episode_ids must contain 1..200 values")
	}
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		refs, err := s.deps.Artifacts.ListLatestByEpisodes(ctx, tx, episodeIDs)
		if err != nil {
			return err
		}
		for _, ref := range refs {
			out[ref.EpisodeID] = true
		}
		return nil
	})
	return out, err
}