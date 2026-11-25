package migrate

import (
	"context"
	"fmt"
	"os"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestUpAgainstRealPostgres(t *testing.T) {
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

	schema := fmt.Sprintf("ms_p05_%d", time.Now().UnixNano())
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = pool.Exec(cleanupCtx, "DROP SCHEMA IF EXISTS "+schema+" CASCADE")
	}()

	target := Target{
		Schema:       schema,
		VersionTable: schema + ".schema_migrations",
		LockKey:      "alert." + schema + ".migrate",
		FS:           threeFiles(schema, "v1"),
	}

	first, err := Up(ctx, pool, target)
	if err != nil {
		t.Fatalf("首次迁移失败: %v", err)
	}
	if len(first.Applied) != 3 || len(first.Skipped) != 0 || len(first.Conflicts) != 0 {
		t.Fatalf("首次结果 = %+v", first)
	}

	second, err := Up(ctx, pool, target)
	if err != nil {
		t.Fatalf("二次迁移失败: %v", err)
	}
	if len(second.Applied) != 0 || len(second.Skipped) != 3 {
		t.Fatalf("二次结果 = %+v", second)
	}

	target.FS = threeFiles(schema, "v2-changed")
	conflicted, err := Up(ctx, pool, target)
	if err != nil {
		t.Fatalf("冲突检测失败: %v", err)
	}
	if len(conflicted.Conflicts) == 0 {
		t.Fatal("修改已应用文件后应报冲突")
	}
	if len(conflicted.Applied) != 0 || len(conflicted.Skipped) != 0 {
		t.Fatalf("冲突时不应执行任何文件: %+v", conflicted)
	}

	applied, expected, err := Status(ctx, pool, target)
	if err != nil {
		t.Fatalf("Status 失败: %v", err)
	}
	if applied != 0 {
		t.Fatalf("applied 应为 0（未执行），实际 %d", applied)
	}
	if expected != 3 {
		t.Fatalf("expected = %d", expected)
	}
}

func threeFiles(schema, marker string) fstest.MapFS {
	return fstest.MapFS{
		"0001_create_t1.sql": &fstest.MapFile{Data: []byte(fmt.Sprintf("CREATE TABLE %s.t1 (id int); -- %s", schema, marker))},
		"0002_create_t2.sql": &fstest.MapFile{Data: []byte(fmt.Sprintf("CREATE TABLE %s.t2 (id int); -- %s", schema, marker))},
		"0003_create_t3.sql": &fstest.MapFile{Data: []byte(fmt.Sprintf("CREATE TABLE %s.t3 (id int); -- %s", schema, marker))},
	}
}
