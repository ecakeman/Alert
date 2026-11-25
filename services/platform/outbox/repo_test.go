package outbox

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRepoAgainstRealPostgres(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("未执行（环境：无 TEST_DATABASE_URL）")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("连接池创建失败: %v", err)
	}
	defer pool.Close()

	schema := fmt.Sprintf("ms_p06_%d", time.Now().UnixNano())
	if err := createSchema(ctx, pool, schema); err != nil {
		t.Fatalf("建表失败: %v", err)
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = pool.Exec(cleanupCtx, "DROP SCHEMA IF EXISTS "+schema+" CASCADE")
	}()

	repo, err := NewRepo(schema)
	if err != nil {
		t.Fatalf("NewRepo 失败: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	first := Record{
		EventID: "11111111-1111-1111-1111-111111111111", EventType: "service.state.changed.v1",
		SchemaVersion: 1, AggregateType: "service", AggregateID: "s-1",
		OccurredAt: now, Payload: []byte(`{"service_id":"s-1"}`),
	}
	dedupe := "dedupe-1"
	first.DedupeKey = &dedupe
	mustAppend(t, ctx, pool, repo, first)

	tx1, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin 失败: %v", err)
	}
	batch, err := repo.Claim(ctx, tx1, "owner-1", now.Add(30*time.Second), 10)
	if err != nil {
		t.Fatalf("Claim 失败: %v", err)
	}
	if len(batch) != 1 || batch[0].EventID != first.EventID {
		t.Fatalf("首次 claim = %+v", batch)
	}

	// 另一个事务在锁持有期间必须跳过该行。
	tx2, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin2 失败: %v", err)
	}
	concurrent, err := repo.Claim(ctx, tx2, "owner-2", now.Add(30*time.Second), 10)
	if err != nil {
		t.Fatalf("并发 Claim 失败: %v", err)
	}
	_ = tx2.Rollback(ctx)
	if len(concurrent) != 0 {
		t.Fatalf("并发 claim 应被 SKIP LOCKED 跳过，实际 %+v", concurrent)
	}
	if err := tx1.Commit(ctx); err != nil {
		t.Fatalf("commit 失败: %v", err)
	}

	// 租约未过期时其他 owner 不能重复领取。
	tx3, _ := pool.Begin(ctx)
	locked, err := repo.Claim(ctx, tx3, "owner-3", now.Add(30*time.Second), 10)
	_ = tx3.Rollback(ctx)
	if err != nil || len(locked) != 0 {
		t.Fatalf("租约期内重复 claim = %+v err=%v", locked, err)
	}

	tx4, _ := pool.Begin(ctx)
	if err := repo.Ack(ctx, tx4, first.EventID, now.Add(time.Second)); err != nil {
		t.Fatalf("Ack 失败: %v", err)
	}
	_ = tx4.Commit(ctx)

	var published *time.Time
	if err := pool.QueryRow(ctx, "SELECT published_at FROM "+schema+".outbox_events WHERE event_id=$1", first.EventID).Scan(&published); err != nil {
		t.Fatalf("读取 published_at 失败: %v", err)
	}
	if published == nil {
		t.Fatal("Ack 后 published_at 仍为空")
	}

	// 失败路径：次数 +1、next_retry_at 前移、published_at 仍为空。
	second := first
	second.EventID = "22222222-2222-2222-2222-222222222222"
	second.DedupeKey = nil
	mustAppend(t, ctx, pool, repo, second)
	tx5, _ := pool.Begin(ctx)
	if _, err := repo.Claim(ctx, tx5, "owner-1", now.Add(30*time.Second), 10); err != nil {
		t.Fatalf("Claim 失败: %v", err)
	}
	_ = tx5.Commit(ctx)

	retryAt := now.Add(10 * time.Second)
	tx6, _ := pool.Begin(ctx)
	if err := repo.Fail(ctx, tx6, second.EventID, retryAt, "kafka down"); err != nil {
		t.Fatalf("Fail 失败: %v", err)
	}
	_ = tx6.Commit(ctx)

	var (
		attempts  int
		nextRetry *time.Time
		pubAt     *time.Time
		lastErr   string
	)
	if err := pool.QueryRow(ctx,
		"SELECT publish_attempts,next_retry_at,published_at,last_error FROM "+schema+".outbox_events WHERE event_id=$1",
		second.EventID).Scan(&attempts, &nextRetry, &pubAt, &lastErr); err != nil {
		t.Fatalf("读取失败行失败: %v", err)
	}
	if attempts != 1 || nextRetry == nil || pubAt != nil || lastErr != "kafka down" {
		t.Fatalf("失败行 = attempts=%d next=%v published=%v err=%q", attempts, nextRetry, pubAt, lastErr)
	}

	// 租约过期后可被重新领取。
	expired := second
	expired.EventID = "33333333-3333-3333-3333-333333333333"
	mustAppend(t, ctx, pool, repo, expired)
	tx7, _ := pool.Begin(ctx)
	if _, err := repo.Claim(ctx, tx7, "owner-x", now.Add(-time.Second), 10); err != nil {
		t.Fatalf("过期租约 claim 失败: %v", err)
	}
	_ = tx7.Commit(ctx)
	tx8, _ := pool.Begin(ctx)
	reclaimed, err := repo.Claim(ctx, tx8, "owner-y", now.Add(30*time.Second), 10)
	_ = tx8.Rollback(ctx)
	if err != nil || len(reclaimed) != 1 || reclaimed[0].EventID != expired.EventID {
		t.Fatalf("过期租约重 claim 失败: %+v err=%v", reclaimed, err)
	}
}

func mustAppend(t *testing.T, ctx context.Context, pool *pgxpool.Pool, repo *Repo, rec Record) {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin 失败: %v", err)
	}
	if err := repo.Append(ctx, tx, rec); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("Append 失败: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit 失败: %v", err)
	}
}

func createSchema(ctx context.Context, pool *pgxpool.Pool, schema string) error {
	if _, err := pool.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		return err
	}
	_, err := pool.Exec(ctx, `CREATE TABLE `+schema+`.outbox_events (
		event_id uuid PRIMARY KEY,
		event_type text NOT NULL,
		schema_version integer NOT NULL DEFAULT 1,
		aggregate_type text NOT NULL,
		aggregate_id text NOT NULL,
		payload jsonb NOT NULL,
		dedupe_key text,
		created_at timestamptz NOT NULL DEFAULT now(),
		published_at timestamptz,
		publish_attempts integer NOT NULL DEFAULT 0,
		next_retry_at timestamptz,
		claim_owner text,
		claim_until timestamptz,
		last_error text NOT NULL DEFAULT ''
	)`)
	return err
}
