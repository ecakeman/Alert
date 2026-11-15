package system

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

func (s *Store) noteViews(ctx context.Context, tx *sql.Tx, serviceID uuid.UUID) error {
	views := []struct {
		scope string
		id    uuid.UUID
	}{
		{"dashboard", uuid.Nil},
		{"services", uuid.Nil},
		{"episodes", uuid.Nil},
		{"service", serviceID},
		{"targets", serviceID},
	}
	for _, view := range views {
		if err := s.bumpRevision(ctx, tx, view.scope, view.id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) bumpRevision(ctx context.Context, tx *sql.Tx, scope string, id uuid.UUID) error {
	var revision int64
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO system.resource_revisions (scope, resource_id, revision)
		VALUES ($1,$2,1)
		ON CONFLICT (scope, resource_id)
		DO UPDATE SET revision = system.resource_revisions.revision + 1
		RETURNING revision`, scope, id).Scan(&revision); err != nil {
		return err
	}
	eventID := uuid.New()
	raw, _ := json.Marshal(map[string]any{
		"scope": scope, "resource_id": id.String(), "revision": revision,
	})
	_, err := tx.ExecContext(ctx, `
		INSERT INTO system.outbox_events
		(outbox_id, event_id, aggregate_type, aggregate_id, event_type, schema_version, payload, next_retry_at, created_at)
		VALUES ($1,$2,'RESOURCE',$3,'RESOURCE_REVISION',1,$4,$5,$5)`,
		uuid.New(), eventID, id, raw, time.Now().UTC())
	return err
}

func (s *Store) PublishByType(ctx context.Context, eventType string, limit int, send func(payload []byte) error) (int, error) {
	if limit <= 0 {
		limit = 16
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `
		SELECT outbox_id, payload FROM system.outbox_events
		WHERE published_at IS NULL AND next_retry_at <= $1 AND event_type=$2
		ORDER BY created_at
		LIMIT $3
		FOR UPDATE SKIP LOCKED`, s.now(), eventType, limit)
	if err != nil {
		return 0, err
	}
	type item struct {
		id      uuid.UUID
		payload []byte
	}
	var batch []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.id, &it.payload); err != nil {
			rows.Close()
			return 0, err
		}
		batch = append(batch, it)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	published := 0
	now := s.now()
	for _, it := range batch {
		if err := send(it.payload); err != nil {
			if _, uerr := tx.ExecContext(ctx, `
				UPDATE system.outbox_events
				SET publish_attempts=publish_attempts+1, next_retry_at=$2
				WHERE outbox_id=$1`, it.id, now.Add(s.RetryBase)); uerr != nil {
				return published, uerr
			}
			continue
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE system.outbox_events SET published_at=$2 WHERE outbox_id=$1`, it.id, now); err != nil {
			return published, err
		}
		published++
	}
	return published, tx.Commit()
}
