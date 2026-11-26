package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// MigrationChecker 断言 registry 本域迁移已应用（version >= 1）。
type MigrationChecker struct {
	Pool    *pgxpool.Pool
	Timeout time.Duration
}

func (c MigrationChecker) Check(ctx context.Context) error {
	if c.Pool == nil {
		return fmt.Errorf("database not configured")
	}
	if c.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.Timeout)
		defer cancel()
	}
	var regclass *string
	if err := c.Pool.QueryRow(ctx, "SELECT to_regclass('registry.schema_migrations')::text").Scan(&regclass); err != nil {
		return fmt.Errorf("registry migration check failed: %w", err)
	}
	if regclass == nil {
		return fmt.Errorf("registry migration not applied")
	}
	var version int64
	if err := c.Pool.QueryRow(ctx, "SELECT coalesce(max(version),0) FROM registry.schema_migrations").Scan(&version); err != nil {
		return fmt.Errorf("registry migration check failed: %w", err)
	}
	if version < 1 {
		return fmt.Errorf("registry migration not applied")
	}
	return nil
}
