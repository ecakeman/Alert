package task

import (
	"context"
	"time"

	"github.com/ecakeman/alert/services/system-core/internal/application/apperr"
	portshared "github.com/ecakeman/alert/services/system-core/internal/application/port/shared"
	porttask "github.com/ecakeman/alert/services/system-core/internal/application/port/task"
	"github.com/ecakeman/alert/services/system-core/internal/domain"
)

const reclaimBatch = 32

// Acquire 领取服务级执行权：domain.CanAcquire 解释拒绝 + 仓储 DB 条件双判。
// 失败返回 SERVICE_BUSY；租约不超过配置的 TASK_LEASE 上限。
func (s *Service) Acquire(ctx context.Context, taskID string, generation int64, lease time.Duration) (bool, error) {
	var allowed bool
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		task, err := s.deps.Tasks.Get(ctx, tx, taskID)
		if err != nil {
			return err
		}
		if task.Generation != generation {
			return apperr.New(domain.CodeStaleGeneration, "stale task generation")
		}
		now := s.deps.Clock.Now()
		if lease <= 0 || lease > s.deps.Lease {
			lease = s.deps.Lease
		}
		existing, err := s.currentOwnership(ctx, tx, task.ServiceID)
		if err != nil {
			return err
		}
		req := domain.Ownership{
			ServiceID:  task.ServiceID,
			TaskID:     task.TaskID,
			Generation: task.Generation,
			LeaseUntil: now.Add(lease).Unix(),
		}
		if decision := domain.CanAcquire(existing, req, now.Unix()); !decision.Allowed {
			return apperr.New(decision.Reason, "service execution is busy")
		}
		ok, err := s.deps.Ownership.Acquire(ctx, tx, porttask.Ownership{
			ServiceID:  req.ServiceID,
			TaskID:     req.TaskID,
			EpisodeID:  task.EpisodeID,
			TaskType:   task.TaskType,
			Generation: req.Generation,
			LeaseUntil: now.Add(lease),
			AcquiredAt: now,
			UpdatedAt:  now,
		})
		if err != nil {
			return err
		}
		if !ok {
			return apperr.New(domain.CodeServiceBusy, "service execution is busy")
		}
		allowed = true
		return nil
	})
	return allowed, err
}

// Release 释放执行权；仅匹配 task_id + generation 的行被删除。
func (s *Service) Release(ctx context.Context, serviceID, taskID string, generation int64) error {
	return s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		_, err := s.deps.Ownership.Release(ctx, tx, serviceID, taskID, generation)
		return err
	})
}

// ReclaimExpired 只回收租约已过期的执行权：释放 + 任务 RUNNING→RETRY_WAIT（不递增代次）。
func (s *Service) ReclaimExpired(ctx context.Context, now time.Time) (int, error) {
	reclaimed := 0
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		expired, err := s.deps.Ownership.ListExpired(ctx, tx, now, reclaimBatch)
		if err != nil {
			return err
		}
		retryWait := string(domain.TaskRetryWait)
		for _, o := range expired {
			released, err := s.deps.Ownership.Release(ctx, tx, o.ServiceID, o.TaskID, o.Generation)
			if err != nil {
				return err
			}
			if !released {
				continue
			}
			updated, err := s.deps.Tasks.UpdateCAS(ctx, tx, o.TaskID, o.Generation,
				[]string{string(domain.TaskRunning)}, porttask.TaskPatch{Status: &retryWait})
			if err != nil {
				return err
			}
			if updated > 0 {
				reclaimed++
			}
		}
		return nil
	})
	return reclaimed, err
}

func (s *Service) currentOwnership(ctx context.Context, tx portshared.Tx, serviceID string) (*domain.Ownership, error) {
	o, err := s.deps.Ownership.Get(ctx, tx, serviceID)
	if err != nil {
		if apperr.Code(err) == apperr.CodeNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &domain.Ownership{
		ServiceID:  o.ServiceID,
		TaskID:     o.TaskID,
		Generation: o.Generation,
		LeaseUntil: o.LeaseUntil.Unix(),
	}, nil
}
