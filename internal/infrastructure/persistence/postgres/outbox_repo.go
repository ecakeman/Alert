package postgres

import (
	"context"
	"time"

	"github.com/ecakeman/alert/internal/application/port"
)

type outboxRepo struct{}

func NewOutboxRepo() port.OutboxRepo { return outboxRepo{} }

const outboxColumns = `event_id,event_type,schema_version,aggregate_type,aggregate_id,payload,dedupe_key,created_at,published_at,publish_attempts,next_retry_at,claim_owner,claim_until,last_error`

func scanOutbox(row scannable) (port.OutboxEvent, error) {
	var e port.OutboxEvent
	if err := row.Scan(&e.EventID, &e.EventType, &e.SchemaVersion, &e.AggregateType, &e.AggregateID,
		&e.Payload, &e.DedupeKey, &e.CreatedAt, &e.PublishedAt, &e.PublishAttempts, &e.NextRetryAt,
		&e.ClaimOwner, &e.ClaimUntil, &e.LastError); err != nil {
		return port.OutboxEvent{}, Translate(err)
	}
	return e, nil
}

func (outboxRepo) Append(ctx context.Context, tx port.Tx, e port.OutboxEvent) error {
	_, err := txFrom(tx).Exec(ctx, `
INSERT INTO system.outbox_events(event_id,event_type,schema_version,aggregate_type,aggregate_id,payload,dedupe_key,created_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8)`,
		e.EventID, e.EventType, e.SchemaVersion, e.AggregateType, e.AggregateID, e.Payload,
		e.DedupeKey, utc(e.CreatedAt))
	return Translate(err)
}

// Claim：短事务内领取待发布事件（SKIP LOCKED），claim_until 为租约上限。
func (outboxRepo) Claim(ctx context.Context, tx port.Tx, owner string, until time.Time, limit int) ([]port.OutboxEvent, error) {
	if limit <= 0 {
		limit = 16
	}
	rows, err := txFrom(tx).Query(ctx, `
UPDATE system.outbox_events SET claim_owner=$1, claim_until=$2
WHERE event_id IN (
  SELECT event_id FROM system.outbox_events
  WHERE published_at IS NULL AND (claim_until IS NULL OR claim_until <= now())
  ORDER BY created_at
  LIMIT $3
  FOR UPDATE SKIP LOCKED
)
RETURNING `+outboxColumns, owner, utc(until), limit)
	if err != nil {
		return nil, Translate(err)
	}
	defer rows.Close()
	var out []port.OutboxEvent
	for rows.Next() {
		e, err := scanOutbox(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, Translate(rows.Err())
}

func (outboxRepo) MarkPublished(ctx context.Context, tx port.Tx, eventID string, at time.Time) error {
	_, err := txFrom(tx).Exec(ctx, `
UPDATE system.outbox_events SET published_at=$2, claim_owner=NULL, claim_until=NULL
WHERE event_id=$1`, eventID, utc(at))
	return Translate(err)
}

func (outboxRepo) MarkFailed(ctx context.Context, tx port.Tx, eventID string, nextRetryAt time.Time, errMsg string) error {
	_, err := txFrom(tx).Exec(ctx, `
UPDATE system.outbox_events SET
  publish_attempts = publish_attempts + 1,
  last_error = $3,
  next_retry_at = $2,
  claim_owner = NULL,
  claim_until = NULL
WHERE event_id=$1`, eventID, utc(nextRetryAt), errMsg)
	return Translate(err)
}
