package task

import (
	"context"
	"encoding/json"
	"time"

	"github.com/ecakeman/alert/services/task-orchestrator/internal/application/apperr"
	portshared "github.com/ecakeman/alert/services/task-orchestrator/internal/application/port/shared"
	porttask "github.com/ecakeman/alert/services/task-orchestrator/internal/application/port/task"
	"github.com/ecakeman/alert/services/task-orchestrator/internal/domain"
)

// RegisterArtifactInput 是产物登记输入。
type RegisterArtifactInput struct {
	Identity        TaskIdentity
	ArtifactType    string
	ArtifactVersion int
	ContentHash     string
	ContentType     string
	StorageKey      string
	SourceRefs      []string
}

// RegisterArtifact 先 fence 再校验类型/追溯/版本/哈希；同版本同 hash 幂等；未知类型 → UNKNOWN_ARTIFACT_TYPE，异 hash → HASH_MISMATCH。
func (s *Service) RegisterArtifact(ctx context.Context, in RegisterArtifactInput) error {
	return s.withFencedTask(ctx, in.Identity, ActionRegisterArtifact,
		func(ctx context.Context, tx portshared.Tx, task porttask.AgentTask, _ time.Time) error {
			ref := domain.ArtifactRef{
				Type:        domain.ArtifactType(in.ArtifactType),
				Version:     int64(in.ArtifactVersion),
				ContentHash: in.ContentHash,
				TaskID:      task.TaskID,
				Generation:  task.Generation,
				IncidentID:  task.EpisodeID,
				SourceRefs:  in.SourceRefs,
			}

			same, err := s.deps.Artifacts.GetByTypeVersion(ctx, tx, task.EpisodeID, in.ArtifactType, in.ArtifactVersion)
			if err == nil {
				if same.ContentHash == in.ContentHash {
					return nil
				}
			} else if apperr.Code(err) != apperr.CodeNotFound {
				return err
			}

			latestVersion := int64(0)
			latestHash := ""
			latest, err := s.deps.Artifacts.GetLatest(ctx, tx, task.EpisodeID, in.ArtifactType)
			if err == nil {
				latestVersion = int64(latest.ArtifactVersion)
				latestHash = latest.ContentHash
			} else if apperr.Code(err) != apperr.CodeNotFound {
				return err
			}
			if err := domain.ValidateArtifactRef(ref, latestVersion, latestHash, task.Generation); err != nil {
				return apperr.Normalize(err)
			}

			sourceRefs, err := json.Marshal(in.SourceRefs)
			if err != nil {
				return err
			}
			record := porttask.ArtifactRef{
				ArtifactRefID:   s.deps.IDs.NewID(),
				EpisodeID:       task.EpisodeID,
				TaskID:          task.TaskID,
				Generation:      task.Generation,
				ArtifactType:    in.ArtifactType,
				ArtifactVersion: in.ArtifactVersion,
				ContentHash:     in.ContentHash,
				ContentType:     in.ContentType,
				StorageKey:      in.StorageKey,
				SourceRefs:      sourceRefs,
				RegisteredAt:    s.deps.Clock.Now(),
				UpdatedAt:       s.deps.Clock.Now(),
			}
			if err := s.deps.Artifacts.Insert(ctx, tx, record); err != nil {
				if apperr.Code(err) == apperr.CodeAlreadyExists {
					return apperr.New(domain.CodeHashMismatch, "artifact version already registered with different hash")
				}
				return err
			}
			return nil
		})
}
