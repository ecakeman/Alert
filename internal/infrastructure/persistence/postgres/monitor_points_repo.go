package postgres

import (
	"context"

	"github.com/ecakeman/alert/internal/application/port"
)

type monitorPointRepo struct{}

func NewMonitorPointRepo() port.MonitorPointRepo { return monitorPointRepo{} }

const monitorPointColumns = `point_seq,service_id,target_id,target_key,target_type,observed_at,received_at,check_status,status_class,error_class,state_fingerprint,latency_ms,probe_generation`

func scanMonitorPoint(row scannable) (port.MonitorPoint, error) {
	var p port.MonitorPoint
	if err := row.Scan(&p.PointSeq, &p.ServiceID, &p.TargetID, &p.TargetKey, &p.TargetType,
		&p.ObservedAt, &p.ReceivedAt, &p.CheckStatus, &p.StatusClass, &p.ErrorClass,
		&p.StateFingerprint, &p.LatencyMS, &p.ProbeGeneration); err != nil {
		return port.MonitorPoint{}, Translate(err)
	}
	return p, nil
}

func (monitorPointRepo) Append(ctx context.Context, tx port.Tx, p port.MonitorPoint) (int64, error) {
	var seq int64
	err := txFrom(tx).QueryRow(ctx, `
INSERT INTO system.service_monitor_points(service_id,target_id,target_key,target_type,observed_at,received_at,check_status,status_class,error_class,state_fingerprint,latency_ms,probe_generation)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING point_seq`,
		p.ServiceID, p.TargetID, p.TargetKey, p.TargetType, utc(p.ObservedAt), utc(p.ReceivedAt),
		p.CheckStatus, p.StatusClass, p.ErrorClass, p.StateFingerprint, p.LatencyMS, p.ProbeGeneration).Scan(&seq)
	if err != nil {
		return 0, Translate(err)
	}
	return seq, nil
}

func (monitorPointRepo) ListByBound(ctx context.Context, tx port.Tx, serviceID string, fromSeq, toSeq int64, limit int) ([]port.MonitorPoint, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := txFrom(tx).Query(ctx, `
SELECT `+monitorPointColumns+` FROM system.service_monitor_points
WHERE service_id=$1 AND point_seq >= $2 AND point_seq <= $3
ORDER BY point_seq
LIMIT $4`, serviceID, fromSeq, toSeq, limit)
	if err != nil {
		return nil, Translate(err)
	}
	defer rows.Close()
	var out []port.MonitorPoint
	for rows.Next() {
		p, err := scanMonitorPoint(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, Translate(rows.Err())
}

func (monitorPointRepo) LatestSeqByService(ctx context.Context, tx port.Tx, serviceID string) (int64, error) {
	var seq int64
	err := txFrom(tx).QueryRow(ctx,
		`SELECT coalesce(max(point_seq),0) FROM system.service_monitor_points WHERE service_id=$1`, serviceID).Scan(&seq)
	if err != nil {
		return 0, Translate(err)
	}
	return seq, nil
}
