package postgres

import (
	"context"

	"github.com/ecakeman/alert/internal/application/port"
)

type appEventRepo struct{}

func NewAppEventRepo() port.AppEventRepo { return appEventRepo{} }

const appEventColumns = `event_record_id,app_event_seq,service_id,episode_id,producer_id,event_id,event_kind,health_status,payload,content_hash,event_time,received_at`

func scanAppEvent(row scannable) (port.AppEvent, error) {
	var e port.AppEvent
	if err := row.Scan(&e.EventRecordID, &e.AppEventSeq, &e.ServiceID, &e.EpisodeID, &e.ProducerID,
		&e.EventID, &e.EventKind, &e.HealthStatus, &e.Payload, &e.ContentHash, &e.EventTime, &e.ReceivedAt); err != nil {
		return port.AppEvent{}, Translate(err)
	}
	return e, nil
}

func (appEventRepo) GetByIdentity(ctx context.Context, tx port.Tx, serviceID, producerID, eventID string) (port.AppEvent, error) {
	return scanAppEvent(txFrom(tx).QueryRow(ctx,
		`SELECT `+appEventColumns+` FROM system.episode_app_events WHERE service_id=$1 AND producer_id=$2 AND event_id=$3`,
		serviceID, producerID, eventID))
}

// Append：在服务行锁下分配 app_event_seq（同服务事件串行），返回 event_record_id。
func (appEventRepo) Append(ctx context.Context, tx port.Tx, e port.AppEvent) (int64, error) {
	conn := txFrom(tx)
	if _, err := conn.Exec(ctx, `SELECT 1 FROM system.services WHERE service_id=$1 FOR UPDATE`, e.ServiceID); err != nil {
		return 0, Translate(err)
	}
	var recordID int64
	err := conn.QueryRow(ctx, `
INSERT INTO system.episode_app_events(app_event_seq,service_id,episode_id,producer_id,event_id,event_kind,health_status,payload,content_hash,event_time,received_at)
VALUES((SELECT coalesce(max(app_event_seq),0)+1 FROM system.episode_app_events WHERE service_id=$1),$1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING event_record_id`,
		e.ServiceID, e.EpisodeID, e.ProducerID, e.EventID, e.EventKind, e.HealthStatus,
		e.Payload, e.ContentHash, utc(e.EventTime), utc(e.ReceivedAt)).Scan(&recordID)
	if err != nil {
		return 0, Translate(err)
	}
	return recordID, nil
}

func (appEventRepo) ListByBound(ctx context.Context, tx port.Tx, serviceID string, fromSeq, toSeq int64, limit int) ([]port.AppEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := txFrom(tx).Query(ctx, `
SELECT `+appEventColumns+` FROM system.episode_app_events
WHERE service_id=$1 AND app_event_seq >= $2 AND app_event_seq <= $3
ORDER BY app_event_seq
LIMIT $4`, serviceID, fromSeq, toSeq, limit)
	if err != nil {
		return nil, Translate(err)
	}
	defer rows.Close()
	var out []port.AppEvent
	for rows.Next() {
		e, err := scanAppEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, Translate(rows.Err())
}