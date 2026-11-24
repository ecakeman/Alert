package postgres

import (
	"context"
	"time"

	portService "github.com/ecakeman/alert/services/system-core/internal/application/port/service"
	portshared "github.com/ecakeman/alert/services/system-core/internal/application/port/shared"
	"github.com/ecakeman/alert/services/system-core/internal/infrastructure/persistence/postgres/model"
)

type targetRepo struct{}

func NewTargetRepo() portService.TargetRepo { return targetRepo{} }

// Create：created_at/updated_at 由 DB DEFAULT now() 接管。
func (targetRepo) Create(ctx context.Context, tx portshared.Tx, t portService.MonitorTarget) error {
	row := model.TargetRowFromApp(t)
	_, err := txFrom(tx).Exec(ctx, `
INSERT INTO system.service_monitor_targets(target_id,service_id,target_key,target_name,address,target_type,config,interval_ms,timeout_ms,failure_threshold,recovery_threshold,is_required,enabled,status,consecutive_failures,consecutive_successes,config_generation)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`,
		row.TargetID, row.ServiceID, row.TargetKey, row.TargetName, row.Address, row.TargetType,
		row.Config, row.IntervalMS, row.TimeoutMS, row.FailureThreshold, row.RecoveryThreshold,
		row.IsRequired, row.Enabled, row.Status, row.ConsecutiveFailures, row.ConsecutiveSuccesses,
		row.ConfigGeneration)
	return Translate(err)
}

func (targetRepo) Get(ctx context.Context, tx portshared.Tx, targetID string) (portService.MonitorTarget, error) {
	row, err := model.TargetRow{}.Scan(txFrom(tx).QueryRow(ctx,
		`SELECT `+model.TargetColumns+` FROM system.service_monitor_targets WHERE target_id=$1`, targetID))
	if err != nil {
		return portService.MonitorTarget{}, Translate(err)
	}
	return row.ToApp(), nil
}

func (targetRepo) ListByService(ctx context.Context, tx portshared.Tx, serviceID string, includeDeleted bool) ([]portService.MonitorTarget, error) {
	rows, err := txFrom(tx).Query(ctx, `
SELECT `+model.TargetColumns+` FROM system.service_monitor_targets
WHERE service_id=$1 AND ($2 OR deleted_at IS NULL)
ORDER BY created_at`, serviceID, includeDeleted)
	if err != nil {
		return nil, Translate(err)
	}
	defer rows.Close()
	var out []portService.MonitorTarget
	for rows.Next() {
		row, err := model.TargetRow{}.Scan(rows)
		if err != nil {
			return nil, Translate(err)
		}
		out = append(out, row.ToApp())
	}
	return out, Translate(rows.Err())
}

func (targetRepo) Patch(ctx context.Context, tx portshared.Tx, targetID string, p portService.TargetPatch) error {
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

// UpdateProbeResult 更新目标聚合计数与状态（探测结果落地）。

func (targetRepo) SoftDelete(ctx context.Context, tx portshared.Tx, targetID string, at time.Time) error {
	_, err := txFrom(tx).Exec(ctx, `
UPDATE system.service_monitor_targets SET deleted_at=$2, enabled=false, updated_at=now()
WHERE target_id=$1 AND deleted_at IS NULL`, targetID, utc(at))
	return Translate(err)
}
