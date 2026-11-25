package postgres

import (
	"context"
	"time"

	"github.com/ecakeman/alert/services/monitor-svc/internal/application/port/state"
	portshared "github.com/ecakeman/alert/services/monitor-svc/internal/application/port/shared"
	"github.com/jackc/pgx/v5"
)

type stateRepo struct{}

func NewStateRepo() state.Repo { return stateRepo{} }

// GetTarget：不存在时返回 UNKNOWN 零值（首次采样即从 UNKNOWN 起步）。
func (stateRepo) GetTarget(ctx context.Context, tx portshared.Tx, targetID string) (state.TargetState, error) {
	out := state.TargetState{TargetID: targetID, Status: "UNKNOWN"}
	err := txFrom(tx).QueryRow(ctx, `
SELECT service_id,status,consecutive_failures,consecutive_successes,last_check_at,updated_at
FROM monitor.monitor_target_state WHERE target_id=$1`, targetID).
		Scan(&out.ServiceID, &out.Status, &out.ConsecutiveFailures, &out.ConsecutiveSuccesses, &out.LastCheckAt, &out.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return state.TargetState{TargetID: targetID, Status: "UNKNOWN"}, nil
		}
		return state.TargetState{}, Translate(err)
	}
	return out, nil
}

func (stateRepo) ListTargets(ctx context.Context, tx portshared.Tx, serviceID string) ([]state.TargetState, error) {
	rows, err := txFrom(tx).Query(ctx, `
SELECT target_id,service_id,status,consecutive_failures,consecutive_successes,last_check_at,updated_at
FROM monitor.monitor_target_state WHERE service_id=$1`, serviceID)
	if err != nil {
		return nil, Translate(err)
	}
	defer rows.Close()
	var out []state.TargetState
	for rows.Next() {
		var s state.TargetState
		if err := rows.Scan(&s.TargetID, &s.ServiceID, &s.Status, &s.ConsecutiveFailures,
			&s.ConsecutiveSuccesses, &s.LastCheckAt, &s.UpdatedAt); err != nil {
			return nil, Translate(err)
		}
		out = append(out, s)
	}
	return out, Translate(rows.Err())
}

func (stateRepo) UpsertTarget(ctx context.Context, tx portshared.Tx, s state.TargetState, at time.Time) error {
	_, err := txFrom(tx).Exec(ctx, `
INSERT INTO monitor.monitor_target_state(
  target_id,service_id,status,consecutive_failures,consecutive_successes,last_check_at,updated_at)
VALUES($1,$2,$3,$4,$5,$6,$7)
ON CONFLICT (target_id) DO UPDATE SET
  service_id=EXCLUDED.service_id, status=EXCLUDED.status,
  consecutive_failures=EXCLUDED.consecutive_failures,
  consecutive_successes=EXCLUDED.consecutive_successes,
  last_check_at=EXCLUDED.last_check_at, updated_at=EXCLUDED.updated_at`,
		s.TargetID, s.ServiceID, s.Status, s.ConsecutiveFailures, s.ConsecutiveSuccesses, utcPtr(s.LastCheckAt), utc(at))
	return Translate(err)
}

func (stateRepo) GetSource(ctx context.Context, tx portshared.Tx, serviceID string) (state.SourceState, error) {
	out := state.SourceState{ServiceID: serviceID, Status: "UNKNOWN"}
	err := txFrom(tx).QueryRow(ctx, `
SELECT status,aggregation_mode,updated_at FROM monitor.monitor_source_state WHERE service_id=$1`, serviceID).
		Scan(&out.Status, &out.AggregationMode, &out.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return state.SourceState{ServiceID: serviceID, Status: "UNKNOWN"}, nil
		}
		return state.SourceState{}, Translate(err)
	}
	return out, nil
}

func (stateRepo) UpsertSource(ctx context.Context, tx portshared.Tx, s state.SourceState, at time.Time) error {
	_, err := txFrom(tx).Exec(ctx, `
INSERT INTO monitor.monitor_source_state(service_id,status,aggregation_mode,updated_at)
VALUES($1,$2,$3,$4)
ON CONFLICT (service_id) DO UPDATE SET
  status=EXCLUDED.status, aggregation_mode=EXCLUDED.aggregation_mode, updated_at=EXCLUDED.updated_at`,
		s.ServiceID, s.Status, s.AggregationMode, utc(at))
	return Translate(err)
}