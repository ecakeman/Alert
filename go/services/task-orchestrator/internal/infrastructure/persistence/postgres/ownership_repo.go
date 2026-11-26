package postgres

import (
	"context"
	"time"

	portshared "github.com/ecakeman/alert/services/task-orchestrator/internal/application/port/shared"
	porttask "github.com/ecakeman/alert/services/task-orchestrator/internal/application/port/task"
	"github.com/ecakeman/alert/services/task-orchestrator/internal/infrastructure/persistence/postgres/model"
)

type ownershipRepo struct{}

func NewOwnershipRepo() porttask.OwnershipRepo { return ownershipRepo{} }

// Acquire：服务级执行权；判定由应用层 domain.CanAcquire 单一负责，
// 仓储只承担行锁 + 唯一约束。无行插入；有行在行锁下无条件覆盖，
// 并发对手的更高代次由调用方任务 CAS（generation 条件）兜底回滚。
func (ownershipRepo) Acquire(ctx context.Context, tx portshared.Tx, o porttask.Ownership) (porttask.AcquireOutcome, error) {
	exec := txFrom(tx)
	row, err := model.OwnershipRow{}.Scan(exec.QueryRow(ctx,
		`SELECT `+model.OwnershipColumns+` FROM task.service_execution_ownership WHERE service_id=$1 FOR UPDATE`,
		o.ServiceID))
	switch {
	case err == nil:
		if row.TaskID == o.TaskID && row.Generation == o.Generation {
			// 同代次幂等：刷新租约后返回 already_held，调用方仍可幂等重投。
			if _, err := exec.Exec(ctx, `
UPDATE task.service_execution_ownership
SET task_id=$2, episode_id=$3, task_type=$4, generation=$5, lease_until=$6, updated_at=now()
WHERE service_id=$1`, o.ServiceID, o.TaskID, o.EpisodeID, o.TaskType, o.Generation, utc(o.LeaseUntil)); err != nil {
				return "", Translate(err)
			}
			return porttask.AcquireAlreadyHeld, nil
		}
		if _, err := exec.Exec(ctx, `
UPDATE task.service_execution_ownership
SET task_id=$2, episode_id=$3, task_type=$4, generation=$5, lease_until=$6, updated_at=now()
WHERE service_id=$1`, o.ServiceID, o.TaskID, o.EpisodeID, o.TaskType, o.Generation, utc(o.LeaseUntil)); err != nil {
			return "", Translate(err)
		}
		return porttask.AcquireAcquired, nil
	case NoRows(err):
		if _, err := exec.Exec(ctx, `
INSERT INTO task.service_execution_ownership
  (service_id,task_id,episode_id,task_type,generation,lease_until,acquired_at,updated_at)
VALUES($1,$2,$3,$4,$5,$6,now(),now())`,
			o.ServiceID, o.TaskID, o.EpisodeID, o.TaskType, o.Generation, utc(o.LeaseUntil)); err != nil {
			return "", Translate(err)
		}
		return porttask.AcquireAcquired, nil
	default:
		return "", Translate(err)
	}
}

func (ownershipRepo) Get(ctx context.Context, tx portshared.Tx, serviceID string) (porttask.Ownership, error) {
	row, err := model.OwnershipRow{}.Scan(txFrom(tx).QueryRow(ctx,
		`SELECT `+model.OwnershipColumns+` FROM task.service_execution_ownership WHERE service_id=$1`, serviceID))
	if err != nil {
		return porttask.Ownership{}, Translate(err)
	}
	return row.ToApp(), nil
}

func (ownershipRepo) Release(ctx context.Context, tx portshared.Tx, serviceID, taskID string, generation int64) (bool, error) {
	tag, err := txFrom(tx).Exec(ctx, `
DELETE FROM task.service_execution_ownership
WHERE service_id=$1 AND task_id=$2 AND generation=$3`, serviceID, taskID, generation)
	if err != nil {
		return false, Translate(err)
	}
	return tag.RowsAffected() == 1, nil
}

// ListExpired 只读返回过期所有权（含任务信息），回收语义由应用层组合。
func (ownershipRepo) ListExpired(ctx context.Context, tx portshared.Tx, now time.Time, limit int) ([]porttask.Ownership, error) {
	if limit <= 0 {
		limit = 32
	}
	rows, err := txFrom(tx).Query(ctx, `
SELECT `+model.OwnershipColumns+` FROM task.service_execution_ownership
WHERE lease_until <= $1 ORDER BY lease_until LIMIT $2`, utc(now), limit)
	if err != nil {
		return nil, Translate(err)
	}
	defer rows.Close()
	var out []porttask.Ownership
	for rows.Next() {
		row, err := model.OwnershipRow{}.Scan(rows)
		if err != nil {
			return nil, Translate(err)
		}
		out = append(out, row.ToApp())
	}
	return out, Translate(rows.Err())
}

func (ownershipRepo) ReclaimExpired(ctx context.Context, tx portshared.Tx, now time.Time) ([]string, error) {
	rows, err := txFrom(tx).Query(ctx, `
SELECT service_id FROM task.service_execution_ownership WHERE lease_until <= $1`, utc(now))
	if err != nil {
		return nil, Translate(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, Translate(err)
		}
		out = append(out, id)
	}
	return out, Translate(rows.Err())
}