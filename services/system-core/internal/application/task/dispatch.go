package task

import (
	"context"
	"fmt"

	"github.com/ecakeman/alert/services/system-core/internal/application/apperr"
	portoutbox "github.com/ecakeman/alert/services/system-core/internal/application/port/outbox"
	portshared "github.com/ecakeman/alert/services/system-core/internal/application/port/shared"
	porttask "github.com/ecakeman/alert/services/system-core/internal/application/port/task"
	"github.com/ecakeman/alert/services/system-core/internal/domain"
)

const dispatchConsumer = "system.task.dispatch.v1"

// Dispatch 事务内幂等 + 领取所有权 + 置 DISPATCHED，事务外提交；未接受回写 RETRY_WAIT。
func (s *Service) Dispatch(ctx context.Context, taskID string, generation int64) (DispatchResult, error) {
	return s.dispatch(ctx, taskID, generation, "")
}

func (s *Service) dispatch(ctx context.Context, taskID string, generation int64, eventID string) (DispatchResult, error) {
	var task porttask.AgentTask
	var skipped bool
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		if eventID != "" {
			first, err := s.deps.Idem.Reserve(ctx, tx, portoutbox.ConsumerIdempotency{
				Consumer:       dispatchConsumer,
				IdempotencyKey: "event:" + eventID,
				EventID:        &eventID,
			})
			if err != nil {
				return err
			}
			if !first {
				skipped = true
				return nil
			}
			first, err = s.deps.Idem.Reserve(ctx, tx, portoutbox.ConsumerIdempotency{
				Consumer:       dispatchConsumer,
				IdempotencyKey: fmt.Sprintf("taskgen:%s:%d", taskID, generation),
				TaskID:         &taskID,
				Generation:     &generation,
			})
			if err != nil {
				return err
			}
			if !first {
				skipped = true
				return nil
			}
		}

		current, err := s.deps.Tasks.GetForUpdate(ctx, tx, taskID)
		if err != nil {
			return err
		}
		if current.Generation != generation {
			return apperr.New(domain.CodeStaleGeneration, "stale task generation")
		}
		if !dispatchable(current.Status) {
			return apperr.New(domain.CodeStateConflict, "task is not dispatchable")
		}

		now := s.deps.Clock.Now()
		existing, err := s.currentOwnership(ctx, tx, current.ServiceID)
		if err != nil {
			return err
		}
		request := domain.Ownership{
			ServiceID:  current.ServiceID,
			TaskID:     current.TaskID,
			Generation: current.Generation,
			LeaseUntil: now.Add(s.deps.Lease).Unix(),
		}
		if decision := domain.CanAcquire(existing, request, now.Unix()); !decision.Allowed {
			skipped = true
			return nil
		}
		acquired, err := s.deps.Ownership.Acquire(ctx, tx, porttask.Ownership{
			ServiceID:  current.ServiceID,
			TaskID:     current.TaskID,
			EpisodeID:  current.EpisodeID,
			TaskType:   current.TaskType,
			Generation: current.Generation,
			LeaseUntil: now.Add(s.deps.Lease),
			AcquiredAt: now,
			UpdatedAt:  now,
		})
		if err != nil {
			return err
		}
		if !acquired {
			skipped = true
			return nil
		}

		leaseUntil := now.Add(s.deps.Lease)
		status := string(domain.TaskDispatched)
		updated, err := s.deps.Tasks.UpdateCAS(ctx, tx, taskID, generation,
			[]string{string(domain.TaskPending), string(domain.TaskRetryWait)},
			porttask.TaskPatch{Status: &status, LeaseUntil: &leaseUntil})
		if err != nil {
			return err
		}
		if updated == 0 {
			skipped = true
			return nil
		}
		current.Status = status
		current.LeaseUntil = &leaseUntil
		task = current
		return nil
	})
	if err != nil {
		return DispatchRejected, err
	}
	if skipped {
		return DispatchRejected, nil
	}

	accepted, err := s.deps.Submit.SubmitTask(ctx, task)
	if err != nil || !accepted {
		if retryErr := s.scheduleDispatchRetry(ctx, task); retryErr != nil {
			return DispatchRetryScheduled, retryErr
		}
		return DispatchRetryScheduled, nil
	}
	return DispatchAccepted, nil
}

func dispatchable(status string) bool {
	return status == string(domain.TaskPending) || status == string(domain.TaskRetryWait)
}

func (s *Service) scheduleDispatchRetry(ctx context.Context, task porttask.AgentTask) error {
	return s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		attempt := task.Attempt + 1
		status := string(domain.TaskRetryWait)
		if attempt >= task.MaxAttempts {
			status = string(domain.TaskFailed)
		}
		nextRetry := s.deps.Clock.Now().Add(s.deps.RetryDelay)
		patch := porttask.TaskPatch{Status: &status, Attempt: &attempt, NextRetryAt: &nextRetry}
		updated, err := s.deps.Tasks.UpdateCAS(ctx, tx, task.TaskID, task.Generation,
			[]string{string(domain.TaskDispatched)}, patch)
		if err != nil {
			return err
		}
		if updated == 0 {
			return apperr.New(domain.CodeStateConflict, "task is no longer dispatched")
		}
		if status == string(domain.TaskFailed) {
			_, err = s.deps.Ownership.Release(ctx, tx, task.ServiceID, task.TaskID, task.Generation)
			return err
		}
		return nil
	})
}
