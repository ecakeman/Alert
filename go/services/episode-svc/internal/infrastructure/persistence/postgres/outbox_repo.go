package postgres

import (
	"context"
	"time"

	"github.com/ecakeman/alert/services/contracts"
	portoutbox "github.com/ecakeman/alert/services/episode-svc/internal/application/port/outbox"
	portshared "github.com/ecakeman/alert/services/episode-svc/internal/application/port/shared"
	"github.com/ecakeman/alert/services/episode-svc/internal/infrastructure/persistence/postgres/model"
)

type outboxRepo struct{}

func NewOutboxRepo() portoutbox.OutboxRepo { return outboxRepo{} }

func (outboxRepo) Append(ctx context.Context, tx portshared.Tx, e contracts.Event, dedupeKey *string) error {
	_, err := txFrom(tx).Exec(ctx, `
INSERT INTO episode.outbox_events(event_id,event_type,schema_version,aggregate_type,aggregate_id,payload,dedupe_key,created_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8)`,
		e.EventID, e.EventType, e.SchemaVersion, e.AggregateType, e.AggregateID,
		e.Payload, dedupeKey, utc(e.OccurredAt))
	return Translate(err)
}

// Claim：短事务内领取待发布事件（SKIP LOCKED），claim_until 为租约上限。
func (outboxRepo) Claim(ctx context.Context, tx portshared.Tx, owner string, until time.Time, limit int) ([]portoutbox.OutboxRecord, error) {
	if limit <= 0 {
		limit = 16
	}
	rows, err := txFrom(tx).Query(ctx, `
UPDATE episode.outbox_events SET claim_owner=$1, claim_until=$2
WHERE event_id IN (
  SELECT event_id FROM episode.outbox_events
  WHERE published_at IS NULL AND (claim_until IS NULL OR claim_until <= now())
  ORDER BY created_at
  LIMIT $3
  FOR UPDATE SKIP LOCKED
)
RETURNING `+model.OutboxColumns, owner, utc(until), limit)
	if err != nil {
		return nil, Translate(err)
	}
	defer rows.Close()
	var out []portoutbox.OutboxRecord
	for rows.Next() {
		row, err := model.OutboxRow{}.Scan(rows)
		if err != nil {
			return nil, Translate(err)
		}
		out = append(out, row.ToApp())
	}
	return out, Translate(rows.Err())
}

func (outboxRepo) MarkPublished(ctx context.Context, tx portshared.Tx, eventID string, at time.Time) error {
	_, err := txFrom(tx).Exec(ctx, `
UPDATE episode.outbox_events SET published_at=$2, claim_owner=NULL, claim_until=NULL
WHERE event_id=$1`, eventID, utc(at))
	return Translate(err)
}

func (outboxRepo) MarkFailed(ctx context.Context, tx portshared.Tx, eventID string, nextRetryAt time.Time, errMsg string) error {
	_, err := txFrom(tx).Exec(ctx, `
UPDATE episode.outbox_events SET
  publish_attempts = publish_attempts + 1,
  last_error = $3,
  next_retry_at = $2,
  claim_owner = NULL,
  claim_until = NULL
WHERE event_id=$1`, eventID, utc(nextRetryAt), errMsg)
	return Translate(err)
}