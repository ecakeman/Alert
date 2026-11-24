package postgres

import (
	"context"

	portmonitor "github.com/ecakeman/alert/internal/application/port/monitor"
	portshared "github.com/ecakeman/alert/internal/application/port/shared"
	"github.com/ecakeman/alert/internal/infrastructure/persistence/postgres/model"
)

type monitorPointRepo struct{}

func NewMonitorPointRepo() portmonitor.MonitorPointRepo { return monitorPointRepo{} }

func (monitorPointRepo) Append(ctx context.Context, tx portshared.Tx, p portmonitor.MonitorPoint) (int64, error) {
	row := model.MonitorPointRowFromApp(p)
	var seq int64
	err := txFrom(tx).QueryRow(ctx, `
INSERT INTO system.service_monitor_points(service_id,target_id,target_key,target_type,observed_at,received_at,check_status,status_class,error_class,state_fingerprint,latency_ms,probe_generation)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING point_seq`,
		row.ServiceID, row.TargetID, row.TargetKey, row.TargetType, utc(row.ObservedAt), utc(row.ReceivedAt),
		row.CheckStatus, row.StatusClass, row.ErrorClass, row.StateFingerprint, row.LatencyMS, row.ProbeGeneration).Scan(&seq)
	if err != nil {
		return 0, Translate(err)
	}
	return seq, nil
}

func (monitorPointRepo) ListByBound(ctx context.Context, tx portshared.Tx, serviceID string, fromSeq, toSeq int64, limit int) ([]portmonitor.MonitorPoint, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := txFrom(tx).Query(ctx, `
SELECT `+model.MonitorPointColumns+` FROM system.service_monitor_points
WHERE service_id=$1 AND point_seq >= $2 AND point_seq <= $3
ORDER BY point_seq
LIMIT $4`, serviceID, fromSeq, toSeq, limit)
	if err != nil {
		return nil, Translate(err)
	}
	defer rows.Close()
	var out []portmonitor.MonitorPoint
	for rows.Next() {
		row, err := model.MonitorPointRow{}.Scan(rows)
		if err != nil {
			return nil, Translate(err)
		}
		out = append(out, row.ToApp())
	}
	return out, Translate(rows.Err())
}

func (monitorPointRepo) LatestSeqByService(ctx context.Context, tx portshared.Tx, serviceID string) (int64, error) {
	var seq int64
	err := txFrom(tx).QueryRow(ctx,
		`SELECT coalesce(max(point_seq),0) FROM system.service_monitor_points WHERE service_id=$1`, serviceID).Scan(&seq)
	if err != nil {
		return 0, Translate(err)
	}
	return seq, nil
}
