package postgres

import (
	"context"

	portepisode "github.com/ecakeman/alert/services/episode-svc/internal/application/port/episode"
	portshared "github.com/ecakeman/alert/services/episode-svc/internal/application/port/shared"
	"github.com/ecakeman/alert/services/episode-svc/internal/infrastructure/persistence/postgres/model"
)

type appEventRepo struct{}

func NewAppEventRepo() portepisode.AppEventRepo { return appEventRepo{} }

func (appEventRepo) GetByIdentity(ctx context.Context, tx portshared.Tx, serviceID, producerID, eventID string) (portepisode.AppEvent, error) {
	row, err := model.AppEventRow{}.Scan(txFrom(tx).QueryRow(ctx,
		`SELECT `+model.AppEventColumns+` FROM episode.episode_app_events WHERE service_id=$1 AND producer_id=$2 AND event_id=$3`,
		serviceID, producerID, eventID))
	if err != nil {
		return portepisode.AppEvent{}, Translate(err)
	}
	return row.ToApp(), nil
}

// Append：在服务级事务咨询锁下分配 app_event_seq（同服务事件串行），返回 event_record_id。
// 使用 advisory lock 而非引用 services 表，保持 episode schema 自洽、无跨域行锁。
func (appEventRepo) Append(ctx context.Context, tx portshared.Tx, e portepisode.AppEvent) (int64, error) {
	conn := txFrom(tx)
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('episode.app_events:'||$1))`, e.ServiceID); err != nil {
		return 0, Translate(err)
	}
	var recordID int64
	err := conn.QueryRow(ctx, `
INSERT INTO episode.episode_app_events(app_event_seq,service_id,episode_id,producer_id,event_id,event_kind,health_status,payload,content_hash,event_time,received_at)
VALUES((SELECT coalesce(max(app_event_seq),0)+1 FROM episode.episode_app_events WHERE service_id=$1),$1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING event_record_id`,
		e.ServiceID, e.EpisodeID, e.ProducerID, e.EventID, e.EventKind, e.HealthStatus,
		e.Payload, e.ContentHash, utc(e.EventTime), utc(e.ReceivedAt)).Scan(&recordID)
	if err != nil {
		return 0, Translate(err)
	}
	return recordID, nil
}

func (appEventRepo) ListByBound(ctx context.Context, tx portshared.Tx, serviceID string, fromSeq, toSeq int64, limit int) ([]portepisode.AppEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := txFrom(tx).Query(ctx, `
SELECT `+model.AppEventColumns+` FROM episode.episode_app_events
WHERE service_id=$1 AND app_event_seq >= $2 AND app_event_seq <= $3
ORDER BY app_event_seq
LIMIT $4`, serviceID, fromSeq, toSeq, limit)
	if err != nil {
		return nil, Translate(err)
	}
	defer rows.Close()
	var out []portepisode.AppEvent
	for rows.Next() {
		row, err := model.AppEventRow{}.Scan(rows)
		if err != nil {
			return nil, Translate(err)
		}
		out = append(out, row.ToApp())
	}
	return out, Translate(rows.Err())
}