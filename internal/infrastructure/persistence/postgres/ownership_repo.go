package postgres

import (
	"context"
	"time"

	"github.com/ecakeman/alert/internal/application/port"
)

type ownershipRepo struct{}

func NewOwnershipRepo() port.OwnershipRepo { return ownershipRepo{} }

const ownershipColumns = `service_id,task_id,episode_id,task_type,generation,lease_until,acquired_at,updated_at`

func scanOwnership(row scannable) (port.Ownership, error) {
	var o port.Ownership
	if err := row.Scan(&o.ServiceID, &o.TaskID, &o.EpisodeID, &o.TaskType, &o.Generation,
		&o.LeaseUntil, &o.AcquiredAt, &o.UpdatedAt); err != nil {
		return port.Ownership{}, Translate(err)
	}
	return o, nil
}

// Acquire：服务级执行权；过期或同 task+generation 幂等时允许接管。
func (ownershipRepo) Acquire(ctx context.Context, tx port.Tx, o port.Ownership) (bool, error) {
	tag, err := txFrom(tx).Exec(ctx, `
INSERT INTO system.service_execution_ownership AS o(service_id,task_id,episode_id,task_type,generation,lease_until,acquired_at,updated_at)
VALUES($1,$2,$3,$4,$5,$6,now(),now())
ON CONFLICT (service_id) DO UPDATE SET
  task_id=EXCLUDED.task_id,
  episode_id=EXCLUDED.episode_id,
  task_type=EXCLUDED.task_type,
  generation=EXCLUDED.generation,
  lease_until=EXCLUDED.lease_until,
  updated_at=now()
WHERE o.lease_until <= now()
   OR (o.task_id = EXCLUDED.task_id AND o.generation = EXCLUDED.generation)`,
		o.ServiceID, o.TaskID, o.EpisodeID, o.TaskType, o.Generation, utc(o.LeaseUntil))
	if err != nil {
		return false, Translate(err)
	}
	return tag.RowsAffected() == 1, nil
}

func (ownershipRepo) Get(ctx context.Context, tx port.Tx, serviceID string) (port.Ownership, error) {
	return scanOwnership(txFrom(tx).QueryRow(ctx,
		`SELECT `+ownershipColumns+` FROM system.service_execution_ownership WHERE service_id=$1`, serviceID))
}

func (ownershipRepo) Release(ctx context.Context, tx port.Tx, serviceID, taskID string, generation int64) (bool, error) {
	tag, err := txFrom(tx).Exec(ctx, `
DELETE FROM system.service_execution_ownership
WHERE service_id=$1 AND task_id=$2 AND generation=$3`, serviceID, taskID, generation)
	if err != nil {
		return false, Translate(err)
	}
	return tag.RowsAffected() == 1, nil
}

func (ownershipRepo) ReclaimExpired(ctx context.Context, tx port.Tx, now time.Time) ([]string, error) {
	rows, err := txFrom(tx).Query(ctx, `
SELECT service_id FROM system.service_execution_ownership WHERE lease_until <= $1`, utc(now))
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
