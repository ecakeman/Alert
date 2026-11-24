package bootstrap

import (
	"context"
	"fmt"

	"github.com/ecakeman/alert/internal/config"
	"github.com/ecakeman/alert/internal/infrastructure/persistence/migrate"
	"github.com/ecakeman/alert/internal/infrastructure/persistence/postgres"
)

// RunMigrate 契约冻结：输出行 "applied=%d skipped=%d conflicts=%d" 逐字节不变；
// conflicts 非空报错；失败退出码由入口 main 承担（成功 0 / 失败 1）。
func RunMigrate(ctx context.Context, cfg config.Config) error {
	if err := cfg.ValidateRuntime(); err != nil {
		return err
	}
	pool, err := postgres.NewPool(ctx, cfg)
	if err != nil {
		return err
	}
	defer pool.Close()

	res, err := migrate.Up(ctx, pool)
	if err != nil {
		return err
	}
	if len(res.Conflicts) > 0 {
		return fmt.Errorf("migration conflicts: %v", res.Conflicts)
	}
	fmt.Printf("applied=%d skipped=%d conflicts=%d\n", len(res.Applied), len(res.Skipped), len(res.Conflicts))
	return nil
}
