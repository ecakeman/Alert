package postgres

import (
	"context"
	"time"

	"github.com/ecakeman/alert/internal/application/port"
	"github.com/ecakeman/alert/internal/infrastructure/persistence/postgres/model"
)

// ListEnabled 返回所有启用且未删除的目标（调度器输入）。
func (targetRepo) ListEnabled(ctx context.Context, tx port.Tx) ([]port.MonitorTarget, error) {
	rows, err := txFrom(tx).Query(ctx, `
SELECT `+model.TargetColumns+` FROM system.service_monitor_targets
WHERE enabled AND deleted_at IS NULL
ORDER BY service_id, target_key`)
	if err != nil {
		return nil, Translate(err)
	}
	defer rows.Close()
	var out []port.MonitorTarget
	for rows.Next() {
		row, err := model.TargetRow{}.Scan(rows)
		if err != nil {
			return nil, Translate(err)
		}
		out = append(out, row.ToApp())
	}
	return out, Translate(rows.Err())
}

// UpdateProbeResult 更新目标聚合计数与状态（探测结果落地）。
func (targetRepo) UpdateProbeResult(ctx context.Context, tx port.Tx, targetID, status string, failures, successes int, lastCheckAt time.Time) error {
	_, err := txFrom(tx).Exec(ctx, `
UPDATE system.service_monitor_targets SET
  status=$2, consecutive_failures=$3, consecutive_successes=$4, last_check_at=$5, updated_at=now()
WHERE target_id=$1 AND deleted_at IS NULL`,
		targetID, status, failures, successes, utc(lastCheckAt))
	return Translate(err)
}
