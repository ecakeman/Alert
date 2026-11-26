package task

import (
	"context"

	"github.com/ecakeman/alert/services/task-orchestrator/internal/application/apperr"
	portshared "github.com/ecakeman/alert/services/task-orchestrator/internal/application/port/shared"
	porttask "github.com/ecakeman/alert/services/task-orchestrator/internal/application/port/task"
)

// ReadArtifact 读取单条产物：故障存在性 → 最新引用 → 正文端口；缺失一律 ARTIFACT_NOT_FOUND。
func (s *Service) ReadArtifact(ctx context.Context, episodeID, artifactType string) (EpisodeArtifactView, error) {
	var ref porttask.ArtifactRef
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		latest, err := s.deps.Artifacts.GetLatest(ctx, tx, episodeID, artifactType)
		if err != nil {
			if apperr.Code(err) == apperr.CodeNotFound {
				return apperr.ArtifactNotFound("artifact not found")
			}
			return err
		}
		ref = latest
		return nil
	})
	if err != nil {
		return EpisodeArtifactView{}, err
	}
	if s.deps.Reader == nil {
		return EpisodeArtifactView{}, apperr.ArtifactNotFound("artifact content is unavailable")
	}
	content, err := s.deps.Reader.ReadArtifact(ctx, ref)
	if err != nil {
		if apperr.Code(err) == apperr.CodeDependencyUnavailable {
			return EpisodeArtifactView{}, err
		}
		return EpisodeArtifactView{}, apperr.ArtifactNotFound("artifact content is unavailable")
	}
	return EpisodeArtifactView{
		ArtifactRefID:   ref.ArtifactRefID,
		EpisodeID:       ref.EpisodeID,
		TaskID:          ref.TaskID,
		Generation:      ref.Generation,
		ArtifactType:    ref.ArtifactType,
		ArtifactVersion: ref.ArtifactVersion,
		ContentHash:     ref.ContentHash,
		ContentType:     content.ContentType,
		StorageKey:      ref.StorageKey,
		RegisteredAt:    ref.RegisteredAt,
		Content:         content.Text,
	}, nil
}

// ListArtifactRefs 返回故障下产物引用元数据（不含正文）。
func (s *Service) ListArtifactRefs(ctx context.Context, episodeID string) ([]porttask.ArtifactRef, error) {
	var out []porttask.ArtifactRef
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		refs, err := s.deps.Artifacts.ListByEpisode(ctx, tx, episodeID)
		if err != nil {
			return err
		}
		out = refs
		return nil
	})
	return out, err
}
