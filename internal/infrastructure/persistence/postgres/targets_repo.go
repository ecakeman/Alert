package postgres

import (
	"context"
	"time"

	"github.com/ecakeman/alert/internal/application/port"
)

type targetRepo struct{}

func NewTargetRepo() port.TargetRepo { return targetRepo{} }

const targetColumns = `target_id,service_id,target_key,target_name,address,target_type,config,interval_ms,timeout_ms,failure_threshold,recovery_threshold,is_required,enabled,status,consecutive_failures,consecutive_successes,last_check_at,config_generation,deleted_at,created_at,updated_at`

func scanTarget(row scannable) (port.MonitorTarget, error) {
	var t port.MonitorTarget
	if err := row.Scan(&t.TargetID, &t.ServiceID, &t.TargetKey, &t.TargetName, &t.Address, &t.TargetType,
		&t.Config, &t.IntervalMS, &t.TimeoutMS, &t.FailureThreshold, &t.RecoveryThreshold, &t.IsRequired,
		&t.Enabled, &t.Status, &t.ConsecutiveFailures, &t.ConsecutiveSuccesses, &t.LastCheckAt,
		&t.ConfigGeneration, &t.DeletedAt, &t.CreatedAt, &t.UpdatedAt); err != nil {
		return port.MonitorTarget{}, Translate(err)
	}
	return t, nil
}

func (targetRepo) Create(ctx context.Context, tx port.Tx, t port.MonitorTarget) error {
	_, err := txFrom(tx).Exec(ctx, `
INSERT INTO system.service_monitor_targets(target_id,service_id,target_key,target_name,address,target_type,config,interval_ms,timeout_ms,failure_threshold,recovery_threshold,is_required,enabled,status,consecutive_failures,consecutive_successes,config_generation,created_at,updated_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)`,
		t.TargetID, t.ServiceID, t.TargetKey, t.TargetName, t.Address, t.TargetType, t.Config,
		t.IntervalMS, t.TimeoutMS, t.FailureThreshold, t.RecoveryThreshold, t.IsRequired, t.Enabled,
		t.Status, t.ConsecutiveFailures, t.ConsecutiveSuccesses, t.ConfigGeneration, utc(t.CreatedAt), utc(t.UpdatedAt))
	return Translate(err)
}

func (targetRepo) Get(ctx context.Context, tx port.Tx, targetID string) (port.MonitorTarget, error) {
	return scanTarget(txFrom(tx).QueryRow(ctx, `SELECT `+targetColumns+` FROM system.service_monitor_targets WHERE target_id=$1`, targetID))
}

func (targetRepo) ListByService(ctx context.Context, tx port.Tx, serviceID string, includeDeleted bool) ([]port.MonitorTarget, error) {
	rows, err := txFrom(tx).Query(ctx, `
SELECT `+targetColumns+` FROM system.service_monitor_targets
WHERE service_id=$1 AND ($2 OR deleted_at IS NULL)
ORDER BY created_at`, serviceID, includeDeleted)
	if err != nil {
		return nil, Translate(err)
	}
	defer rows.Close()
	var out []port.MonitorTarget
	for rows.Next() {
		t, err := scanTarget(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, Translate(rows.Err())
}

func (targetRepo) Patch(ctx context.Context, tx port.Tx, targetID string, p port.TargetPatch) error {
	_, err := txFrom(tx).Exec(ctx, `
UPDATE system.service_monitor_targets SET
  target_name = COALESCE($2, target_name),
  address = COALESCE($3, address),
  config = COALESCE($4, config),
  interval_ms = COALESCE($5, interval_ms),
  timeout_ms = COALESCE($6, timeout_ms),
  failure_threshold = COALESCE($7, failure_threshold),
  recovery_threshold = COALESCE($8, recovery_threshold),
  is_required = COALESCE($9, is_required),
  enabled = COALESCE($10, enabled),
  config_generation = config_generation + 1,
  updated_at = now()
WHERE target_id=$1 AND deleted_at IS NULL`,
		targetID, p.TargetName, p.Address, p.Config, p.IntervalMS, p.TimeoutMS,
		p.FailureThreshold, p.RecoveryThreshold, p.IsRequired, p.Enabled)
	return Translate(err)
}

func (targetRepo) SoftDelete(ctx context.Context, tx port.Tx, targetID string, at time.Time) error {
	_, err := txFrom(tx).Exec(ctx, `
UPDATE system.service_monitor_targets SET deleted_at=$2, enabled=false, updated_at=now()
WHERE target_id=$1 AND deleted_at IS NULL`, targetID, utc(at))
	return Translate(err)
}