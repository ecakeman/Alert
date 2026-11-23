package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ecakeman/alert/internal/infrastructure/persistence/migrate"
)

// PGChecker 通过 Ping 判断数据库可达；detail 只输出稳定短语。
type PGChecker struct {
	pool    *pgxpool.Pool
	timeout time.Duration
}

func NewPGChecker(pool *pgxpool.Pool, timeout time.Duration) *PGChecker {
	return &PGChecker{pool: pool, timeout: timeout}
}

func (c *PGChecker) Name() string { return "pg" }

func (c *PGChecker) Check(ctx context.Context) error {
	pingCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	if err := c.pool.Ping(pingCtx); err != nil {
		return errors.New("database unreachable")
	}
	return nil
}

// MigrationChecker 判断迁移版本是否与文件期望一致。
type MigrationChecker struct {
	pool    *pgxpool.Pool
	timeout time.Duration
}

func NewMigrationChecker(pool *pgxpool.Pool, timeout time.Duration) *MigrationChecker {
	return &MigrationChecker{pool: pool, timeout: timeout}
}

func (c *MigrationChecker) Name() string { return "migrations" }

func (c *MigrationChecker) Check(ctx context.Context) error {
	checkCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	applied, expected, err := migrate.Status(checkCtx, c.pool)
	if err != nil {
		return errors.New("schema status unavailable")
	}
	if applied < expected {
		return errors.New("schema not migrated")
	}
	if applied > expected {
		return errors.New("schema version conflict")
	}
	return nil
}