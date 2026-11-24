package task

import (
	"context"
	"time"

	"github.com/ecakeman/alert/services/system-core/internal/application/apperr"
	portshared "github.com/ecakeman/alert/services/system-core/internal/application/port/shared"
	porttask "github.com/ecakeman/alert/services/system-core/internal/application/port/task"
	"github.com/ecakeman/alert/services/system-core/internal/domain"
)

type processedVersionAdvancer interface {
	AdvanceProcessedVersion(ctx context.Context, tx portshared.Tx, episodeID string, target int64) (int64, error)
}

// Complete 只写任务成功与 processed_input_version，随后释放所有权；不触碰故障状态列。
func (s *Service) Complete(ctx context.Context, id TaskIdentity) error {
	return s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		task, err := s.deps.Tasks.GetForUpdate(ctx, tx, id.TaskID)
		if err != nil {
			return err
		}
		now := s.deps.Clock.Now()
		if task.Status == string(domain.TaskSucceeded) && task.Generation == id.Generation {
			return nil
		}
		if err := s.fence(task, id, ActionReportResult, now); err != nil {
			return err
		}
		status := string(domain.TaskSucceeded)
		updated, err := s.deps.Tasks.UpdateCAS(ctx, tx, task.TaskID, task.Generation,
			[]string{string(domain.TaskRunning)},
			porttask.TaskPatch{Status: &status, FinishedAt: &now})
		if err != nil {
			return err
		}
		if updated == 0 {
			return apperr.New(domain.CodeStateConflict, "task is not running")
		}
		advancer, ok := s.deps.Episodes.(processedVersionAdvancer)
		if !ok {
			return apperr.New(apperr.CodeDependencyUnavailable, "episode progress advance is unavailable")
		}
		if _, err := advancer.AdvanceProcessedVersion(ctx, tx, task.EpisodeID, task.InputVersionTo); err != nil {
			return err
		}
		_, err = s.deps.Ownership.Release(ctx, tx, task.ServiceID, task.TaskID, task.Generation)
		return err
	})
}

// ReportFailure 按可重试/不可重试分流：可重试且未达上限 → RETRY_WAIT，否则 FAILED 并释放。
func (s *Service) ReportFailure(ctx context.Context, id TaskIdentity, class FailureClass, code, message string) error {
	outcome := domain.OutcomeRetryableFailure
	if class == FailureFatal {
		outcome = domain.OutcomeFatalFailure
	}
	return s.withFencedTask(ctx, id, ActionReportFailure, func(ctx context.Context, tx portshared.Tx, task porttask.AgentTask, now time.Time) error {
		effect := domain.ApplyTaskResult(outcome, task.Attempt, task.MaxAttempts, now.Unix(), int64(s.deps.RetryDelay/time.Second))
		status := string(effect.Status)
		attempt := effect.Attempt
		patch := porttask.TaskPatch{Status: &status, Attempt: &attempt, ErrorCode: &code, ErrorMessage: &message}
		if effect.NextRetryAt > 0 {
			nextRetry := time.Unix(effect.NextRetryAt, 0).UTC()
			patch.NextRetryAt = &nextRetry
		}
		updated, err := s.deps.Tasks.UpdateCAS(ctx, tx, task.TaskID, task.Generation,
			[]string{string(domain.TaskRunning)}, patch)
		if err != nil {
			return err
		}
		if updated == 0 {
			return apperr.New(domain.CodeStateConflict, "task is not running")
		}
		if effect.Status == domain.TaskFailed {
			_, err = s.deps.Ownership.Release(ctx, tx, task.ServiceID, task.TaskID, task.Generation)
			return err
		}
		return nil
	})
}
