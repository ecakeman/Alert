package postgres

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ecakeman/alert/internal/application/apperr"
	"github.com/ecakeman/alert/internal/application/port"
)

func intPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("未执行（环境：无 TEST_DATABASE_URL）")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
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

func sampleService(id, key string) port.Service {
	now := time.Now().UTC()
	return port.Service{
		ServiceID: id, ServiceKey: key, Name: key, Enabled: true,
		MonitorAggregationMode: "ALL_REQUIRED", AuthorityProducerID: "p1",
		CreatedAt: now, UpdatedAt: now,
	}
}

func TestRepoDuplicateServiceKey(t *testing.T) {
	pool := intPool(t)
	uow := NewUnitOfWork(pool)
	ctx := context.Background()
	err := uow.WithinTx(ctx, func(ctx context.Context, tx port.Tx) error {
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
	err := uow.WithinTx(ctx, func(ctx context.Context, tx port.Tx) error {
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
	err := uow.WithinTx(ctx, func(ctx context.Context, tx port.Tx) error {
		svcID := "11111111-1111-1111-1111-111111111111"
		if err := NewServiceRepo().Create(ctx, tx, sampleService(svcID, "s1")); err != nil {
			return err
		}
		now := time.Now().UTC()
		ep := NewEpisodeRepo()
		if err := ep.Create(ctx, tx, port.Episode{
			EpisodeID: "22222222-2222-2222-2222-222222222222", ServiceID: svcID,
			Status: "ACTIVE", StartedBy: "MONITOR", StartedAt: now, CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			return err
		}
		return ep.Create(ctx, tx, port.Episode{
			EpisodeID: "33333333-3333-3333-3333-333333333333", ServiceID: svcID,
			Status: "ACTIVE", StartedBy: "APP", StartedAt: now, CreatedAt: now, UpdatedAt: now,
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
	err := uow.WithinTx(ctx, func(ctx context.Context, tx port.Tx) error {
		ev := NewOutboxRepo()
		if err := ev.Append(ctx, tx, port.OutboxEvent{
			EventID: "44444444-4444-4444-4444-444444444444", EventType: "test.v1", SchemaVersion: 1,
			AggregateType: "service", AggregateID: "x", Payload: []byte(`{}`), CreatedAt: now,
		}); err != nil {
			return err
		}
		first, err := ev.Claim(ctx, tx, "ownerA", now.Add(time.Minute), 1)
		if err != nil {
			return err
		}
		if len(first) != 1 {
			t.Fatalf("expected 1 claimed, got %d", len(first))
		}
		second, err := ev.Claim(ctx, tx, "ownerB", now.Add(time.Minute), 1)
		if err != nil {
			return err
		}
		if len(second) != 0 {
			t.Fatalf("expected 0 on second claim while claimed, got %d", len(second))
		}
		return errors.New("rollback")
	})
	if err == nil || err.Error() != "rollback" {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestRepoTransactionRollback(t *testing.T) {
	pool := intPool(t)
	uow := NewUnitOfWork(pool)
	ctx := context.Background()
	now := time.Now().UTC()
	boom := errors.New("boom")
	err := uow.WithinTx(ctx, func(ctx context.Context, tx port.Tx) error {
		if err := NewServiceRepo().Create(ctx, tx, sampleService("55555555-5555-5555-5555-555555555555", "rollback-key")); err != nil {
			return err
		}
		if err := NewOutboxRepo().Append(ctx, tx, port.OutboxEvent{
			EventID: "66666666-6666-6666-6666-666666666666", EventType: "t", SchemaVersion: 1,
			AggregateType: "a", AggregateID: "b", Payload: []byte(`{}`), CreatedAt: now,
		}); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("expected boom, got %v", err)
	}
	err = uow.WithinTx(ctx, func(ctx context.Context, tx port.Tx) error {
		_, err := NewServiceRepo().GetByKey(ctx, tx, "rollback-key")
		return err
	})
	if appCode(err) != apperr.CodeNotFound {
		t.Fatalf("expected rolled back (NOT_FOUND), got %v", err)
	}
}
