package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	portshared "github.com/ecakeman/alert/services/episode-svc/internal/application/port/shared"
)

// UnitOfWork 实现 portshared.UnitOfWork：一次用例一个事务（默认 READ COMMITTED），
// 对序列化失败 40001 / 死锁 40P01 做一次重试。
type UnitOfWork struct {
	pool *pgxpool.Pool
}

func NewUnitOfWork(pool *pgxpool.Pool) *UnitOfWork {
	return &UnitOfWork{pool: pool}
}

func (u *UnitOfWork) WithinTx(ctx context.Context, fn func(context.Context, portshared.Tx) error) error {
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		lastErr = u.runOnce(ctx, fn)
		if lastErr == nil {
			return nil
		}
		if !isRetryable(lastErr) {
			break
		}
	}
	return lastErr
}

func (u *UnitOfWork) runOnce(ctx context.Context, fn func(context.Context, portshared.Tx) error) error {
	tx, err := u.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Translate(err)
	}
	if err := fn(ctx, tx); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return Translate(err)
	}
	return nil
}

func isRetryable(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "40001" || pgErr.Code == "40P01"
	}
	return false
}