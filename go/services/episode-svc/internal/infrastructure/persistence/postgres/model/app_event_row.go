package model

import (
	"time"

	portepisode "github.com/ecakeman/alert/services/episode-svc/internal/application/port/episode"
)

// AppEventColumns 是 episode.episode_app_events 的完整列清单（唯一来源）。
const AppEventColumns = `event_record_id,app_event_seq,service_id,episode_id,producer_id,event_id,event_kind,health_status,payload,content_hash,event_time,received_at`

type AppEventRow struct {
	EventRecordID int64
	AppEventSeq   int64
	ServiceID     string
	EpisodeID     *string
	ProducerID    string
	EventID       string
	EventKind     string
	HealthStatus  string
	Payload       []byte
	ContentHash   string
	EventTime     time.Time
	ReceivedAt    time.Time
}

func (r AppEventRow) Scan(row Scannable) (AppEventRow, error) {
	if err := row.Scan(&r.EventRecordID, &r.AppEventSeq, &r.ServiceID, &r.EpisodeID, &r.ProducerID,
		&r.EventID, &r.EventKind, &r.HealthStatus, &r.Payload, &r.ContentHash, &r.EventTime, &r.ReceivedAt); err != nil {
		return AppEventRow{}, err
	}
	return r, nil
}

func (r AppEventRow) ToApp() portepisode.AppEvent {
	return portepisode.AppEvent{
		EventRecordID: r.EventRecordID, AppEventSeq: r.AppEventSeq,
		ServiceID: r.ServiceID, EpisodeID: r.EpisodeID,
		ProducerID: r.ProducerID, EventID: r.EventID,
		EventKind: r.EventKind, HealthStatus: r.HealthStatus,
		Payload: r.Payload, ContentHash: r.ContentHash,
		EventTime: r.EventTime, ReceivedAt: r.ReceivedAt,
	}
}