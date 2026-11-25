package task

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ecakeman/alert/services/system-core/internal/application/apperr"
	portshared "github.com/ecakeman/alert/services/system-core/internal/application/port/shared"
	porttask "github.com/ecakeman/alert/services/system-core/internal/application/port/task"
	"github.com/ecakeman/alert/services/system-core/internal/contracts"
	"github.com/ecakeman/alert/services/system-core/internal/domain"
)

// RetryDiagnosis 仅允许最近 DIAGNOSIS 任务处于终态时新建 generation+1 任务。
func (s *Service) RetryDiagnosis(ctx context.Context, episodeID string) (TaskView, error) {
	return s.retry(ctx, episodeID, TaskTypeDiagnosis)
}

// RetryArchive 需 RESOLVED + 双产物 + hash 一致；新建 ARCHIVE 任务（代次 +1）。
func (s *Service) RetryArchive(ctx context.Context, episodeID string) (TaskView, error) {
	return s.retry(ctx, episodeID, TaskTypeArchive)
}

func (s *Service) retry(ctx context.Context, episodeID, taskType string) (TaskView, error) {
	var view TaskView
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		episode, err := s.deps.Episodes.Get(ctx, tx, episodeID)
		if err != nil {
			return err
		}
		active, err := s.deps.Tasks.ListActiveByEpisode(ctx, tx, episodeID)
		if err != nil {
			return err
		}
		if len(active) > 0 {
			return apperr.New(domain.CodeStateConflict, "an active task exists for this episode")
		}

		if taskType == TaskTypeArchive {
			if episode.Status != string(domain.IncidentResolved) {
				return apperr.New(domain.CodeNotResolved, "episode is not resolved")
			}
			artifacts, err := s.archiveArtifactsWithin(ctx, tx, episodeID)
			if err != nil {
				return err
			}
			if ok, code := domain.CanArchive(domain.IncidentResolved, episodeID, artifacts); !ok {
				return apperr.New(code, "archive preconditions are not met")
			}
		}

		recent, err := s.deps.Tasks.ListByEpisode(ctx, tx, episodeID)
		if err != nil {
			return err
		}
		var last *porttask.AgentTask
		for i := range recent {
			if recent[i].TaskType == taskType {
				last = &recent[i]
				break
			}
		}
		if last == nil {
			return apperr.New(domain.CodeStateConflict, "no previous task can be retried")
		}
		plan := domain.PlanManualRetry(last.Generation, domain.TaskStatus(last.Status))
		if plan.Err != nil {
			return apperr.Normalize(plan.Err)
		}
		if !plan.OK {
			return apperr.New(domain.CodeStateConflict, "manual retry only allowed from terminal state")
		}

		now := s.deps.Clock.Now()
		from := last.InputVersionFrom
		if from <= 0 {
			from = 1
		}
		to := episode.InputVersion
		if to < from {
			to = from
		}
		if taskType == TaskTypeArchive {
			from = to
		}
		task := porttask.AgentTask{
			TaskID:            s.deps.IDs.NewID(),
			EpisodeID:         episode.EpisodeID,
			ServiceID:         episode.ServiceID,
			TaskType:          taskType,
			Status:            string(domain.TaskPending),
			Generation:        plan.NewGeneration,
			MaxAttempts:       s.deps.MaxAttempts,
			InputVersionFrom:  from,
			InputVersionTo:    to,
			MonitorWatermark:  episode.MonitorWatermark,
			AppEventWatermark: episode.AppEventWatermark,
			DeadlineAt:        now.Add(s.deps.Deadline),
			CreatedAt:         now,
			UpdatedAt:         now,
		}
		if err := s.deps.Tasks.Create(ctx, tx, task); err != nil {
			if apperr.Code(err) == apperr.CodeAlreadyExists {
				return apperr.New(domain.CodeStateConflict, "an active task exists for this episode")
			}
			return err
		}
		if err := s.appendDispatchEvent(ctx, tx, task, now); err != nil {
			return err
		}
		view = toTaskView(task)
		return nil
	})
	return view, err
}

func (s *Service) appendDispatchEvent(ctx context.Context, tx portshared.Tx, task porttask.AgentTask, now time.Time) error {
	payload, err := json.Marshal(map[string]any{
		"task_id":    task.TaskID,
		"generation": task.Generation,
	})
	if err != nil {
		return err
	}
	dedupe := fmt.Sprintf("task.dispatch:%s:%d", task.TaskID, task.Generation)
	return s.deps.Outbox.Append(ctx, tx, contracts.Event{
		EventID:       s.deps.IDs.NewID(),
		EventType:     contracts.EventTaskDispatchRequested,
		SchemaVersion: 1,
		AggregateType: "task",
		AggregateID:   task.TaskID,
		OccurredAt:    now,
		Payload:       payload,
	}, &dedupe)
}

func (s *Service) archiveArtifactsWithin(ctx context.Context, tx portshared.Tx, episodeID string) ([]domain.ArtifactStatus, error) {
	var out []domain.ArtifactStatus
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
	return out, nil
}
