package postgres

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/ecakeman/alert/internal/config"
)

// 纯函数用例：非法 DSN 返回错误且不泄露 DSN 片段。
func TestNewPoolInvalidDSN(t *testing.T) {
	c, _ := config.Load(func(string) string { return "" })
	c.DatabaseURL = "not-a-valid-dsn"
	_, err := NewPool(context.Background(), c)
	if err == nil {
		t.Fatal("expected error for invalid DSN")
	}
	if strings.Contains(err.Error(), "://") || strings.Contains(err.Error(), "not-a-valid-dsn") {
		t.Fatalf("error must not echo DSN: %v", err)
	}
}

// 真实连接用例：依赖 TEST_DATABASE_URL，缺失时跳过。
func TestNewPoolReal(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("未执行（环境：无 TEST_DATABASE_URL）")
	}
	c, _ := config.Load(func(string) string { return "" })
	c.DatabaseURL = dsn
	pool, err := NewPool(context.Background(), c)
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}
	defer pool.Close()
}