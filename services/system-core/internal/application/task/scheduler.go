package task

import (
	"context"

	portepisode "github.com/ecakeman/alert/services/system-core/internal/application/port/episode"
	portshared "github.com/ecakeman/alert/services/system-core/internal/application/port/shared"
	porttask "github.com/ecakeman/alert/services/system-core/internal/application/port/task"
	"github.com/ecakeman/alert/services/system-core/internal/domain"
)

const schedulerBatch = 32

// EnsureDiagnosisTasks 扫描 ACTIVE 故障：有未处理输入且无活动任务时创建 DIAGNOSIS。
func (s *Service) EnsureDiagnosisTasks(ctx context.Context, limit int) (int, error) {
	candidates, err := s.listEpisodes(ctx, string(domain.IncidentActive), limit)
	if err != nil {
		return 0, err
	}
	created := 0
	for _, episode := range candidates {
		if episode.ProcessedInputVersion >= episode.InputVersion {
			continue
		}
		active, err := s.activeTasks(ctx, episode.EpisodeID)
		if err != nil || len(active) > 0 {
			continue
		}
		view, reused, err := s.Create(ctx, CreateInput{
			EpisodeID: episode.EpisodeID,
			ServiceID: episode.ServiceID,
			TaskType:  TaskTypeDiagnosis,
			Source:    SourceEpisodeActive,
		})
		if err != nil {
			continue
		}
		if !reused && view.TaskID != "" {
			created++
		}
	}
	return created, nil
}

// EnsureArchiveTasks 扫描 RESOLVED 故障：双产物齐全且 hash 一致、无活动任务时创建 ARCHIVE。
func (s *Service) EnsureArchiveTasks(ctx context.Context, limit int) (int, error) {
	candidates, err := s.listEpisodes(ctx, string(domain.IncidentResolved), limit)
	if err != nil {
		return 0, err
	}
	created := 0
	for _, episode := range candidates {
		active, err := s.activeTasks(ctx, episode.EpisodeID)
		if err != nil || len(active) > 0 {
			continue
		}
		artifacts, err := s.archiveArtifacts(ctx, episode.EpisodeID)
		if err != nil {
			continue
		}
		if ok, _ := domain.CanArchive(domain.IncidentResolved, episode.EpisodeID, artifacts); !ok {
			continue
		}
		view, reused, err := s.Create(ctx, CreateInput{
			EpisodeID: episode.EpisodeID,
			ServiceID: episode.ServiceID,
			TaskType:  TaskTypeArchive,
			Source:    SourceEpisodeResolved,
		})
		if err != nil {
			continue
		}
		if !reused && view.TaskID != "" {
			created++
		}
	}
	return created, nil
}

func (s *Service) listEpisodes(ctx context.Context, status string, limit int) ([]portepisode.Episode, error) {
	if limit <= 0 || limit > schedulerBatch {
		limit = schedulerBatch
	}
	var out []portepisode.Episode
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		list, err := s.deps.Episodes.List(ctx, tx, portepisode.EpisodeFilter{Status: status, Limit: limit})
		if err != nil {
			return err
		}
		out = list
		return nil
	})
	return out, err
}

func (s *Service) activeTasks(ctx context.Context, episodeID string) ([]porttask.AgentTask, error) {
	var out []porttask.AgentTask
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		list, err := s.deps.Tasks.ListActiveByEpisode(ctx, tx, episodeID)
		if err != nil {
			return err
		}
		out = list
		return nil
	})
	return out, err
}

func (s *Service) archiveArtifacts(ctx context.Context, episodeID string) ([]domain.ArtifactStatus, error) {
	var out []domain.ArtifactStatus
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		for _, artifactType := range []domain.ArtifactType{domain.ArtifactInvestigation, domain.ArtifactDiagnosisReport} {
			ref, err := s.deps.Artifacts.GetLatest(ctx, tx, episodeID, string(artifactType))
			if err != nil {
				out = append(out, domain.ArtifactStatus{Type: artifactType})
				continue
			}
			out = append(out, domain.ArtifactStatus{
				Type:           domain.ArtifactType(ref.ArtifactType),
				Version:        int64(ref.ArtifactVersion),
				ContentHash:    ref.ContentHash,
				RegisteredHash: ref.ContentHash,
				IncidentID:     ref.EpisodeID,
			})
		}
		return nil
	})
	return out, err
}
