package bootstrap

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	portshared "github.com/ecakeman/alert/services/system-core/internal/application/port/shared"
	"github.com/ecakeman/alert/services/system-core/internal/config"
	"github.com/ecakeman/alert/services/system-core/internal/infrastructure"
	"github.com/ecakeman/alert/services/system-core/internal/infrastructure/persistence/migrate"
	"github.com/ecakeman/alert/services/system-core/internal/infrastructure/persistence/postgres"
)

// Shared 是公共资源束：池的唯一持有者；api 与 worker 装配共享。
type Shared struct {
	Cfg   config.Config
	Pool  *pgxpool.Pool
	UOW   portshared.UnitOfWork
	Clock portshared.Clock
	IDs   portshared.IDGen
}

// NewShared 按冻结顺序构造：ValidateRuntime → 建池 → MigrateOnStart 可选迁移 → UOW/Clock/IDs。
func NewShared(ctx context.Context, cfg config.Config) (*Shared, error) {
	if err := cfg.ValidateRuntime(); err != nil {
		return nil, err
	}

	pool, err := postgres.NewPool(ctx, cfg)
	if err != nil {
		return nil, err
	}

	if cfg.MigrateOnStart {
		res, err := migrate.Up(ctx, pool)
		if err != nil {
			pool.Close()
			return nil, err
		}
		if len(res.Conflicts) > 0 {
			pool.Close()
			return nil, fmt.Errorf("migration conflicts: %v", res.Conflicts)
		}
	}

	return &Shared{
		Cfg: cfg, Pool: pool,
		UOW: postgres.NewUnitOfWork(pool), Clock: infrastructure.RealClock{}, IDs: infrastructure.IDGen{},
	}, nil
}
