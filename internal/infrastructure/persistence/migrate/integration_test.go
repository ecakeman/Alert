package migrate

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("未执行（环境：无 TEST_DATABASE_URL）")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect test db: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// 空迁移阶段：Up 幂等（二次 no-op），Status 返回 applied=expected。
func TestUpIdempotent(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	res1, err := Up(ctx, pool)
	if err != nil {
		t.Fatalf("first Up: %v", err)
	}
	if len(res1.Conflicts) != 0 {
		t.Fatalf("unexpected conflicts: %+v", res1.Conflicts)
	}

	res2, err := Up(ctx, pool)
	if err != nil {
		t.Fatalf("second Up: %v", err)
	}
	if len(res2.Applied) != 0 {
		t.Fatalf("second Up should be no-op, applied %+v", res2.Applied)
	}

	applied, expected, err := Status(ctx, pool)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if applied != expected {
		t.Fatalf("status applied=%d expected=%d", applied, expected)
	}
}