package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ecakeman/alert/services/system-core/internal/application/apperr"
	portepisode "github.com/ecakeman/alert/services/system-core/internal/application/port/episode"
	portService "github.com/ecakeman/alert/services/system-core/internal/application/port/service"
	portshared "github.com/ecakeman/alert/services/system-core/internal/application/port/shared"
	"github.com/ecakeman/alert/services/system-core/internal/contracts"
)

func intPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("未执行（环境：无 TEST_DATABASE_URL）")
	}
	cfg, cfgErr := pgxpool.ParseConfig(dsn)
	if cfgErr != nil {
		t.Fatalf("parse test db config: %v", cfgErr)
	}
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func appCode(err error) string {
	var ae *apperr.Error
	if errors.As(err, &ae) {
		return ae.Code
	}
	return ""
}

func sampleService(id, key string) portService.Service {
	return portService.Service{
		ServiceID: id, ServiceKey: key, Name: key, Enabled: true,
		MonitorAggregationMode: "ALL_REQUIRED", AuthorityProducerID: "p1",
	}
}

func TestRepoDuplicateServiceKey(t *testing.T) {
	pool := intPool(t)
	uow := NewUnitOfWork(pool)
	ctx := context.Background()
	err := uow.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		repo := NewServiceRepo()
		if err := repo.Create(ctx, tx, sampleService("11111111-1111-1111-1111-111111111111", "dup")); err != nil {
			return err
		}
		return repo.Create(ctx, tx, sampleService("22222222-2222-2222-2222-222222222222", "dup"))
	})
	if appCode(err) != apperr.CodeAlreadyExists {
		t.Fatalf("expected ALREADY_EXISTS, got %v", err)
	}
}

func TestRepoNotFound(t *testing.T) {
	pool := intPool(t)
	uow := NewUnitOfWork(pool)
	ctx := context.Background()
	err := uow.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		_, err := NewServiceRepo().Get(ctx, tx, "00000000-0000-0000-0000-000000000000")
		return err
	})
	if appCode(err) != apperr.CodeNotFound {
		t.Fatalf("expected NOT_FOUND, got %v", err)
	}
}

func TestRepoOneActiveEpisode(t *testing.T) {
	pool := intPool(t)
	uow := NewUnitOfWork(pool)
	ctx := context.Background()
	err := uow.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		svcID := "11111111-1111-1111-1111-111111111111"
		if err := NewServiceRepo().Create(ctx, tx, sampleService(svcID, "s1")); err != nil {
			return err
		}
		now := time.Now().UTC()
		ep := NewEpisodeRepo()
		if err := ep.Create(ctx, tx, portepisode.Episode{
			EpisodeID: "22222222-2222-2222-2222-222222222222", ServiceID: svcID,
			Status: "ACTIVE", StartedBy: "MONITOR", StartedAt: now, UpdatedAt: now,
		}); err != nil {
			return err
		}
		return ep.Create(ctx, tx, portepisode.Episode{
			EpisodeID: "33333333-3333-3333-3333-333333333333", ServiceID: svcID,
			Status: "ACTIVE", StartedBy: "APP", StartedAt: now, UpdatedAt: now,
		})
	})
	if appCode(err) != apperr.CodeAlreadyExists {
		t.Fatalf("expected one-active conflict, got %v", err)
	}
}

func TestRepoOutboxClaimExclusive(t *testing.T) {
	pool := intPool(t)
	uow := NewUnitOfWork(pool)
	ctx := context.Background()
	now := time.Now().UTC()
	eventID := "44444444-4444-4444-4444-444444444444"
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, "DELETE FROM system.outbox_events WHERE event_id=$1", eventID)
	})

	// 发布者 A：写入事件并领取（独立事务，提交后行处于已领取状态）。
	err := uow.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		ev := NewOutboxRepo()
		if err := ev.Append(ctx, tx, contracts.Event{
			EventID: eventID, EventType: "test.v1", SchemaVersion: 1,
			AggregateType: "service", AggregateID: "x", Payload: []byte(`{}`), OccurredAt: now,
		}, nil); err != nil {
			return err
		}
		first, err := ev.Claim(ctx, tx, "ownerA", now.Add(time.Minute), 1)
		if err != nil {
			return err
		}
		if len(first) != 1 {
			return fmt.Errorf("expected 1 claimed, got %d", len(first))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("first claim tx: %v", err)
	}

	// 发布者 B：独立事务竞争；库中可能有其他历史 pending 行，Claim 领到它们是
	// 正确行为，这里只关心 A 已领取且未过期的行不被抢走。
	if err := uow.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		_, err := NewOutboxRepo().Claim(ctx, tx, "ownerB", now.Add(time.Minute), 1)
		return err
	}); err != nil {
		t.Fatalf("second claim tx: %v", err)
	}

	// 排他断言（事务外执行，避免 fn 内 Goexit 死锁 pool.Close）。
	var owner *string
	err = uow.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		return tx.(pgx.Tx).QueryRow(ctx,
			"SELECT claim_owner FROM system.outbox_events WHERE event_id=$1", eventID).Scan(&owner)
	})
	if err != nil {
		t.Fatalf("read claim owner: %v", err)
	}
	if owner != nil && *owner != "ownerA" {
		t.Fatalf("claim stolen: owner=%s", *owner)
	}
}

func TestRepoTransactionRollback(t *testing.T) {
	pool := intPool(t)
	uow := NewUnitOfWork(pool)
	ctx := context.Background()
	now := time.Now().UTC()
	boom := errors.New("boom")
	err := uow.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		if err := NewServiceRepo().Create(ctx, tx, sampleService("55555555-5555-5555-5555-555555555555", "rollback-key")); err != nil {
			return err
		}
		if err := NewOutboxRepo().Append(ctx, tx, contracts.Event{
			EventID: "66666666-6666-6666-6666-666666666666", EventType: "t", SchemaVersion: 1,
			AggregateType: "a", AggregateID: "b", Payload: []byte(`{}`), OccurredAt: now,
		}, nil); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("expected boom, got %v", err)
	}
	err = uow.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		_, err := NewServiceRepo().GetByKey(ctx, tx, "rollback-key")
		return err
	})
	if appCode(err) != apperr.CodeNotFound {
		t.Fatalf("expected rolled back (NOT_FOUND), got %v", err)
	}
}
