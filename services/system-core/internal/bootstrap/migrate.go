package bootstrap

import (
	"context"
	"fmt"

	platformmigrate "github.com/ecakeman/alert/services/platform/migrate"

	"github.com/ecakeman/alert/services/system-core/internal/config"
	"github.com/ecakeman/alert/services/system-core/internal/infrastructure/persistence/postgres"
	"github.com/ecakeman/alert/services/system-core/migrations"
)

// RunMigrate 契约冻结：输出行 "applied=%d skipped=%d conflicts=%d" 逐字节不变；
// conflicts 非空报错；失败退出码由入口 main 承担（成功 0 / 失败 1）。
// 迁移实现已切到 platform/migrate 的参数化 runner（schema=system，lock key 与历史一致）。
func RunMigrate(ctx context.Context, cfg config.Config) error {
	if err := cfg.ValidateRuntime(); err != nil {
		return err
	}
	pool, err := postgres.NewPool(ctx, cfg)
	if err != nil {
		return err
	}
	defer pool.Close()

	res, err := platformmigrate.Up(ctx, pool, platformmigrate.Target{
		Schema:       "system",
		VersionTable: "system.schema_migrations",
		LockKey:      "alert.system.migrate",
		FS:           migrations.FS,
	})
	if err != nil {
		return err
	}
	if len(res.Conflicts) > 0 {
		return fmt.Errorf("migration conflicts: %v", res.Conflicts)
	}
	fmt.Printf("applied=%d skipped=%d conflicts=%d\n", len(res.Applied), len(res.Skipped), len(res.Conflicts))
	return nil
}