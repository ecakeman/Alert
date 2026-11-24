package model

import (
	"time"

	"github.com/ecakeman/alert/internal/application/port"
	"github.com/ecakeman/alert/internal/contracts"
)

// OutboxColumns 是 system.outbox_events 的完整列清单（唯一来源）。
const OutboxColumns = `event_id,event_type,schema_version,aggregate_type,aggregate_id,payload,dedupe_key,created_at,published_at,publish_attempts,next_retry_at,claim_owner,claim_until,last_error`

// OutboxRow 是 system.outbox_events 行模型：事件信封 + 投递控制字段。
// created_at ↔ Event.OccurredAt（列名不变，语义单一来源）。
type OutboxRow struct {
	Event           contracts.Event
	DedupeKey       *string
	PublishedAt     *time.Time
	PublishAttempts int
	NextRetryAt     *time.Time
	ClaimOwner      *string
	ClaimUntil      *time.Time
	LastError       string
}

// Scan 从一行读取 OutboxRow。
func (r OutboxRow) Scan(row Scannable) (OutboxRow, error) {
	if err := row.Scan(&r.Event.EventID, &r.Event.EventType, &r.Event.SchemaVersion,
		&r.Event.AggregateType, &r.Event.AggregateID, &r.Event.Payload,
		&r.DedupeKey, &r.Event.OccurredAt, &r.PublishedAt, &r.PublishAttempts,
		&r.NextRetryAt, &r.ClaimOwner, &r.ClaimUntil, &r.LastError); err != nil {
		return OutboxRow{}, err
	}
	return r, nil
}

// ToApp 转换为 application 值类型。
func (r OutboxRow) ToApp() port.OutboxRecord {
	return port.OutboxRecord{
		Event:           r.Event,
		DedupeKey:       r.DedupeKey,
		PublishedAt:     r.PublishedAt,
		PublishAttempts: r.PublishAttempts,
		NextRetryAt:     r.NextRetryAt,
		ClaimOwner:      r.ClaimOwner,
		ClaimUntil:      r.ClaimUntil,
		LastError:       r.LastError,
	}
}
