package outbox

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// DBTX 抽象连接池与事务，使同一份 SQL 既可在事务内（Append）也可在短事务内（Claim/Ack/Fail）执行。
type DBTX interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

var schemaRe = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)

// Repo 是某域私有 schema 的 outbox 仓储；列与 system.outbox_events 逐列一致。
type Repo struct {
	schema string
}

// NewRepo 校验 schema 标识符并返回仓储。
func NewRepo(schema string) (*Repo, error) {
	if !schemaRe.MatchString(schema) {
		return nil, fmt.Errorf("outbox repo: invalid schema")
	}
	return &Repo{schema: schema}, nil
}

// Schema 返回仓储所属 schema。
func (r *Repo) Schema() string { return r.schema }

func (r *Repo) table() string { return r.schema + ".outbox_events" }

// Append 在调用方事务内追加一条待发布事件；dedupe_key 可空且唯一。
// created_at 承载事件发生时刻（列名与历史表一致）。
func (r *Repo) Append(ctx context.Context, db DBTX, rec Record) error {
	_, err := db.Exec(ctx, `INSERT INTO `+r.table()+`(
		event_id,event_type,schema_version,aggregate_type,aggregate_id,payload,dedupe_key,created_at
	) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`,
		rec.EventID, rec.EventType, rec.SchemaVersion, rec.AggregateType, rec.AggregateID,
		rec.Payload, rec.DedupeKey, rec.OccurredAt.UTC())
	return err
}

// Claim 在短事务内领取待发布事件（FOR UPDATE SKIP LOCKED），claim_until 为租约上限。
func (r *Repo) Claim(ctx context.Context, db DBTX, owner string, until time.Time, limit int) ([]Record, error) {
	if limit <= 0 {
		limit = 16
	}
	rows, err := db.Query(ctx, `UPDATE `+r.table()+` SET claim_owner=$1, claim_until=$2
	WHERE event_id IN (
		SELECT event_id FROM `+r.table()+`
		WHERE published_at IS NULL AND (claim_until IS NULL OR claim_until <= now())
		ORDER BY created_at
		LIMIT $3
		FOR UPDATE SKIP LOCKED
	)
	RETURNING `+columns, owner, until.UTC(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Record
	for rows.Next() {
		rec, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

// Ack 标记发布成功并释放租约。
func (r *Repo) Ack(ctx context.Context, db DBTX, eventID string, at time.Time) error {
	_, err := db.Exec(ctx, `UPDATE `+r.table()+` SET
		published_at=$2, claim_owner=NULL, claim_until=NULL
	WHERE event_id=$1`, eventID, at.UTC())
	return err
}

// Fail 记录发布失败：递增次数、写入错误与下次重试时间，不删除行。
func (r *Repo) Fail(ctx context.Context, db DBTX, eventID string, nextRetryAt time.Time, errMsg string) error {
	_, err := db.Exec(ctx, `UPDATE `+r.table()+` SET
		publish_attempts = publish_attempts + 1,
		last_error = $3,
		next_retry_at = $2,
		claim_owner = NULL,
		claim_until = NULL
	WHERE event_id=$1`, eventID, nextRetryAt.UTC(), errMsg)
	return err
}

const columns = `event_id,event_type,schema_version,aggregate_type,aggregate_id,payload,
	dedupe_key,created_at,published_at,publish_attempts,next_retry_at,claim_owner,claim_until,last_error`

type scanner interface {
	Scan(dest ...any) error
}

func scan(s scanner) (Record, error) {
	var (
		rec        Record
		payload    []byte
		dedupeKey  *string
		occurred   time.Time
		published  *time.Time
		nextRetry  *time.Time
		claimOwner *string
		claimUntil *time.Time
	)
	if err := s.Scan(
		&rec.EventID, &rec.EventType, &rec.SchemaVersion, &rec.AggregateType, &rec.AggregateID,
		&payload, &dedupeKey, &occurred, &published, &rec.PublishAttempts, &nextRetry,
		&claimOwner, &claimUntil, &rec.LastError,
	); err != nil {
		return Record{}, err
	}
	rec.OccurredAt = occurred
	rec.Payload = payload
	rec.DedupeKey = dedupeKey
	rec.PublishedAt = published
	rec.NextRetryAt = nextRetry
	rec.ClaimOwner = claimOwner
	rec.ClaimUntil = claimUntil
	return rec, nil
}
