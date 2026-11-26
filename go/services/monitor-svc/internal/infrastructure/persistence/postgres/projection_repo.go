package postgres

import (
	"context"
	"time"

	"github.com/ecakeman/alert/services/monitor-svc/internal/application/port/projection"
	portshared "github.com/ecakeman/alert/services/monitor-svc/internal/application/port/shared"
	"github.com/ecakeman/alert/services/monitor-svc/internal/infrastructure/persistence/postgres/model"
)

type projectionRepo struct{}

func NewProjectionRepo() projection.Repo { return projectionRepo{} }

// Upsert：config_generation 乱序旧值不覆盖新值；目标重新出现时清空 deleted_at；保留自产 last_probe_at。
func (projectionRepo) Upsert(ctx context.Context, tx portshared.Tx, t projection.Target, syncedAt time.Time) error {
	_, err := txFrom(tx).Exec(ctx, `
INSERT INTO monitor.monitor_target_projection(
  target_id,service_id,target_key,target_type,address,interval_ms,timeout_ms,
  failure_threshold,recovery_threshold,is_required,enabled,config_generation,
  monitor_aggregation_mode,deleted_at,synced_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,NULL,$14)
ON CONFLICT (target_id) DO UPDATE SET
  service_id=EXCLUDED.service_id, target_key=EXCLUDED.target_key, target_type=EXCLUDED.target_type,
  address=EXCLUDED.address, interval_ms=EXCLUDED.interval_ms, timeout_ms=EXCLUDED.timeout_ms,
  failure_threshold=EXCLUDED.failure_threshold, recovery_threshold=EXCLUDED.recovery_threshold,
  is_required=EXCLUDED.is_required, enabled=EXCLUDED.enabled,
  config_generation=EXCLUDED.config_generation, monitor_aggregation_mode=EXCLUDED.monitor_aggregation_mode,
  deleted_at=NULL, synced_at=EXCLUDED.synced_at
WHERE EXCLUDED.config_generation >= monitor.monitor_target_projection.config_generation`,
		t.TargetID, t.ServiceID, t.TargetKey, t.TargetType, t.Address, t.IntervalMS, t.TimeoutMS,
		t.FailureThreshold, t.RecoveryThreshold, t.IsRequired, t.Enabled, t.ConfigGeneration,
		t.MonitorAggregationMode, utc(syncedAt))
	return Translate(err)
}

func (projectionRepo) MarkDeleted(ctx context.Context, tx portshared.Tx, serviceID string, keepTargetIDs []string, at time.Time) error {
	_, err := txFrom(tx).Exec(ctx, `
UPDATE monitor.monitor_target_projection SET deleted_at=$2, synced_at=$2
WHERE service_id=$1 AND deleted_at IS NULL AND NOT (target_id = ANY($3::uuid[]))`,
		serviceID, utc(at), keepTargetIDs)
	return Translate(err)
}

func (projectionRepo) MarkServiceDeleted(ctx context.Context, tx portshared.Tx, serviceID string, at time.Time) error {
	_, err := txFrom(tx).Exec(ctx, `
UPDATE monitor.monitor_target_projection SET deleted_at=$2, synced_at=$2
WHERE service_id=$1 AND deleted_at IS NULL`, serviceID, utc(at))
	return Translate(err)
}

func (projectionRepo) Get(ctx context.Context, tx portshared.Tx, targetID string) (projection.Target, error) {
	row := txFrom(tx).QueryRow(ctx, `SELECT `+model.ProjectionColumns+` FROM monitor.monitor_target_projection WHERE target_id=$1`, targetID)
	r, err := model.ProjectionRow{}.Scan(row)
	if err != nil {
		return projection.Target{}, Translate(err)
	}
	return r.ToApp(), nil
}

func (projectionRepo) ListByService(ctx context.Context, tx portshared.Tx, serviceID string, includeDeleted bool) ([]projection.Target, error) {
	query := `SELECT ` + model.ProjectionColumns + ` FROM monitor.monitor_target_projection WHERE service_id=$1`
	if !includeDeleted {
		query += ` AND deleted_at IS NULL`
	}
	query += ` ORDER BY target_key`
	rows, err := txFrom(tx).Query(ctx, query, serviceID)
	if err != nil {
		return nil, Translate(err)
	}
	defer rows.Close()
	var out []projection.Target
	for rows.Next() {
		row, err := model.ProjectionRow{}.Scan(rows)
		if err != nil {
			return nil, Translate(err)
		}
		out = append(out, row.ToApp())
	}
	return out, Translate(rows.Err())
}

// ListDue 返回启用且到期（last_probe_at 为空或已超 interval）的未删除投影。
func (projectionRepo) ListDue(ctx context.Context, tx portshared.Tx, now time.Time) ([]projection.Target, error) {
	rows, err := txFrom(tx).Query(ctx, `
SELECT `+model.ProjectionColumns+` FROM monitor.monitor_target_projection
WHERE enabled AND deleted_at IS NULL
  AND (last_probe_at IS NULL OR last_probe_at + (interval_ms * interval '1 millisecond') <= $1)
ORDER BY service_id, target_key`, utc(now))
	if err != nil {
		return nil, Translate(err)
	}
	defer rows.Close()
	var out []projection.Target
	for rows.Next() {
		row, err := model.ProjectionRow{}.Scan(rows)
		if err != nil {
			return nil, Translate(err)
		}
		out = append(out, row.ToApp())
	}
	return out, Translate(rows.Err())
}

func (projectionRepo) UpdateLastProbeAt(ctx context.Context, tx portshared.Tx, targetID string, at time.Time) error {
	_, err := txFrom(tx).Exec(ctx, `
UPDATE monitor.monitor_target_projection SET last_probe_at=$2
WHERE target_id=$1 AND deleted_at IS NULL`, targetID, utc(at))
	return Translate(err)
}