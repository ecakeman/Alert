package task

import (
	"context"
	"time"

	"github.com/ecakeman/alert/services/task-orchestrator/internal/application/apperr"
	portshared "github.com/ecakeman/alert/services/task-orchestrator/internal/application/port/shared"
	porttask "github.com/ecakeman/alert/services/task-orchestrator/internal/application/port/task"
	"github.com/ecakeman/alert/services/task-orchestrator/internal/domain"
)

// Started 置 RUNNING + started_at + 续租；同代次重复开始幂等。
func (s *Service) Started(ctx context.Context, id TaskIdentity) error {
	return s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		task, err := s.deps.Tasks.GetForUpdate(ctx, tx, id.TaskID)
		if err != nil {
			return err
		}
		now := s.deps.Clock.Now()
		if task.Status == string(domain.TaskRunning) && task.Generation == id.Generation && leaseValid(task, id, now) {
			return nil
		}
		if err := s.fence(task, id, ActionStarted, now); err != nil {
			return err
		}
		status := string(domain.TaskRunning)
		leaseUntil := now.Add(s.deps.Lease)
		updated, err := s.deps.Tasks.UpdateCAS(ctx, tx, task.TaskID, task.Generation,
			[]string{string(domain.TaskPending), string(domain.TaskDispatched)},
			porttask.TaskPatch{Status: &status, StartedAt: &now, LeaseUntil: &leaseUntil})
		if err != nil {
			return err
		}
		if updated == 0 {
			return apperr.New(domain.CodeStateConflict, "task state changed before start")
		}
		return nil
	})
}

// Heartbeat 只续 lease_until，不改变状态。
func (s *Service) Heartbeat(ctx context.Context, id TaskIdentity) error {
	return s.withFencedTask(ctx, id, ActionHeartbeat, func(ctx context.Context, tx portshared.Tx, task porttask.AgentTask, now time.Time) error {
		leaseUntil := now.Add(s.deps.Lease)
		updated, err := s.deps.Tasks.UpdateCAS(ctx, tx, task.TaskID, task.Generation,
			[]string{string(domain.TaskRunning)}, porttask.TaskPatch{LeaseUntil: &leaseUntil})
		if err != nil {
			return err
		}
		if updated == 0 {
			return apperr.New(domain.CodeStateConflict, "task is no longer running")
		}
		return nil
	})
}

// Abandon 主动放弃：RUNNING → RETRY_WAIT，同代次不递增。
func (s *Service) Abandon(ctx context.Context, id TaskIdentity) error {
	return s.withFencedTask(ctx, id, ActionAbandon, func(ctx context.Context, tx portshared.Tx, task porttask.AgentTask, now time.Time) error {
		status := string(domain.TaskRetryWait)
		nextRetry := now.Add(s.deps.RetryDelay)
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

func (s *Service) withFencedTask(
	ctx context.Context,
	id TaskIdentity,
	action TaskAction,
	fn func(context.Context, portshared.Tx, porttask.AgentTask, time.Time) error,
) error {
	return s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		task, err := s.deps.Tasks.GetForUpdate(ctx, tx, id.TaskID)
		if err != nil {
			return err
		}
		now := s.deps.Clock.Now()
		if err := s.fence(task, id, action, now); err != nil {
			return err
		}
		return fn(ctx, tx, task, now)
	})
}
