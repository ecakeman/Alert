package task

import (
	"context"
	"log"
	"time"

	"github.com/ecakeman/alert/services/system-core/internal/application/apperr"
	portshared "github.com/ecakeman/alert/services/system-core/internal/application/port/shared"
	porttask "github.com/ecakeman/alert/services/system-core/internal/application/port/task"
	"github.com/ecakeman/alert/services/system-core/internal/domain"
)

const reconcileBatch = 32

// Reconcile 修复四类中断：已派发无受理、运行中失租、事件滞留、待重试到期；
// 终态显式跳过，单轮每类有界（limit=32）。
func (s *Service) Reconcile(ctx context.Context, now time.Time) (ReconcileSummary, error) {
	var summary ReconcileSummary

	if stale, err := s.staleDispatched(ctx, now); err != nil {
		summary.Failures = append(summary.Failures, err.Error())
	} else {
		for _, task := range stale {
			if terminal(task.Status) {
				continue
			}
			if _, err := s.Dispatch(ctx, task.TaskID, task.Generation); err != nil {
				summary.Failures = append(summary.Failures, err.Error())
				continue
			}
			summary.StaleDispatched++
		}
	}

	if expired, err := s.runningLeaseExpired(ctx, now); err != nil {
		summary.Failures = append(summary.Failures, err.Error())
	} else {
		for _, task := range expired {
			if terminal(task.Status) {
				continue
			}
			if !task.DeadlineAt.IsZero() && task.DeadlineAt.Before(now) {
				if err := s.failExpiredTask(ctx, task); err != nil {
					summary.Failures = append(summary.Failures, err.Error())
				}
				continue
			}
			if err := s.releaseExpiredLease(ctx, task); err != nil {
				summary.Failures = append(summary.Failures, err.Error())
				continue
			}
			summary.LeaseExpired++
		}
	}

	if pending, err := s.dispatchPending(ctx); err != nil {
		summary.Failures = append(summary.Failures, err.Error())
	} else {
		for _, task := range pending {
			if terminal(task.Status) {
				continue
			}
			if _, err := s.Dispatch(ctx, task.TaskID, task.Generation); err != nil {
				summary.Failures = append(summary.Failures, err.Error())
				continue
			}
			summary.StaleDispatched++
		}
	}

	if due, err := s.dueRetry(ctx, now); err != nil {
		summary.Failures = append(summary.Failures, err.Error())
	} else {
		for _, task := range due {
			if terminal(task.Status) {
				continue
			}
			if !domain.CanAutoPromote(domain.TaskStatus(task.Status), retryAtUnix(task.NextRetryAt), now.Unix()) {
				continue
			}
			if err := s.PromoteRetry(ctx, task.TaskID, task.Generation); err != nil {
				summary.Failures = append(summary.Failures, err.Error())
				continue
			}
			summary.RetryPromoted++
		}
	}

	s.logSummary(summary)
	return summary, nil
}

// PromoteRetry 到期提升：代次 +1 并写入新一代次的派发事件。
func (s *Service) PromoteRetry(ctx context.Context, taskID string, generation int64) error {
	return s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		updated, err := s.deps.Tasks.PromoteRetry(ctx, tx, taskID, generation)
		if err != nil {
			return err
		}
		if updated == 0 {
			return apperr.New(domain.CodeStateConflict, "task is not due for retry")
		}
		task, err := s.deps.Tasks.Get(ctx, tx, taskID)
		if err != nil {
			return err
		}
		return s.appendDispatchEvent(ctx, tx, task, s.deps.Clock.Now())
	})
}

func (s *Service) staleDispatched(ctx context.Context, now time.Time) ([]porttask.AgentTask, error) {
	var out []porttask.AgentTask
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		list, err := s.deps.Tasks.ListStaleDispatched(ctx, tx, now.Add(-2*s.deps.Lease), reconcileBatch)
		if err != nil {
			return err
		}
		out = list
		return nil
	})
	return out, err
}

func (s *Service) runningLeaseExpired(ctx context.Context, now time.Time) ([]porttask.AgentTask, error) {
	var out []porttask.AgentTask
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		list, err := s.deps.Tasks.ListRunningLeaseExpired(ctx, tx, now, reconcileBatch)
		if err != nil {
			return err
		}
		out = list
		return nil
	})
	return out, err
}

func (s *Service) dispatchPending(ctx context.Context) ([]porttask.AgentTask, error) {
	var out []porttask.AgentTask
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		list, err := s.deps.Tasks.ListDispatchPending(ctx, tx, reconcileBatch)
		if err != nil {
			return err
		}
		out = list
		return nil
	})
	return out, err
}

func (s *Service) dueRetry(ctx context.Context, now time.Time) ([]porttask.AgentTask, error) {
	var out []porttask.AgentTask
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		list, err := s.deps.Tasks.ListDueRetry(ctx, tx, now, reconcileBatch)
		if err != nil {
			return err
		}
		out = list
		return nil
	})
	return out, err
}

func (s *Service) releaseExpiredLease(ctx context.Context, task porttask.AgentTask) error {
	return s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		if _, err := s.deps.Ownership.Release(ctx, tx, task.ServiceID, task.TaskID, task.Generation); err != nil {
			return err
		}
		status := string(domain.TaskRetryWait)
		nextRetry := s.deps.Clock.Now().Add(s.deps.RetryDelay)
		updated, err := s.deps.Tasks.UpdateCAS(ctx, tx, task.TaskID, task.Generation,
			[]string{string(domain.TaskRunning)},
			porttask.TaskPatch{Status: &status, NextRetryAt: &nextRetry})
		if err != nil {
			return err
		}
		if updated == 0 {
			return apperr.New(domain.CodeStateConflict, "task is no longer running")
		}
		return nil
	})
}

func (s *Service) failExpiredTask(ctx context.Context, task porttask.AgentTask) error {
	return s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		status := string(domain.TaskFailed)
		code := "DEADLINE_EXCEEDED"
		message := "task deadline exceeded"
		updated, err := s.deps.Tasks.UpdateCAS(ctx, tx, task.TaskID, task.Generation,
			[]string{string(domain.TaskRunning)},
			porttask.TaskPatch{Status: &status, ErrorCode: &code, ErrorMessage: &message})
		if err != nil {
			return err
		}
		if updated == 0 {
			return apperr.New(domain.CodeStateConflict, "task is no longer running")
		}
		_, err = s.deps.Ownership.Release(ctx, tx, task.ServiceID, task.TaskID, task.Generation)
		return err
	})
}

func (s *Service) logSummary(summary ReconcileSummary) {
	log.Printf("reconcile done by_type{stale=%d lease=%d retry=%d} errors=%d limit=%d",
		summary.StaleDispatched, summary.LeaseExpired, summary.RetryPromoted,
		len(summary.Failures), reconcileBatch)
}

func terminal(status string) bool {
	return status == string(domain.TaskSucceeded) || status == string(domain.TaskFailed)
}

func retryAtUnix(at *time.Time) int64 {
	if at == nil {
		return 0
	}
	return at.Unix()
}
