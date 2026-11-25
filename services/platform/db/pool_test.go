package db

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func TestConfigValidateMapsToLoadError(t *testing.T) {
	_, err := New(context.Background(), Config{URL: ""})
	var le *LoadError
	if !errors.As(err, &le) || le.Var != "DATABASE_URL" {
		t.Fatalf("期望 DATABASE_URL LoadError，实际 %v", err)
	}

	cases := []struct {
		name    string
		cfg     Config
		varName string
	}{
		{"max conns", Config{URL: "postgres://u:p@127.0.0.1:5432/db", MaxConns: 101}, "DB_MAX_CONNS"},
		{"min conns", Config{URL: "postgres://u:p@127.0.0.1:5432/db", MaxConns: 2, MinConns: 3}, "DB_MIN_CONNS"},
		{"connect timeout", Config{URL: "postgres://u:p@127.0.0.1:5432/db", ConnectTimeout: -time.Second}, "DB_CONNECT_TIMEOUT"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := New(context.Background(), tc.cfg)
			var le *LoadError
			if !errors.As(err, &le) || le.Var != tc.varName {
				t.Fatalf("期望 %s LoadError，实际 %v", tc.varName, err)
			}
		})
	}
}

func TestNewDoesNotWaitForDatabase(t *testing.T) {
	cfg := Config{
		URL:             "postgres://alert:s3cret@127.0.0.1:1/alert?sslmode=disable",
		MaxConns:        2,
		ConnectTimeout:  200 * time.Millisecond,
		MaxRetryBackoff: 500 * time.Millisecond,
	}
	start := time.Now()
	pool, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatalf("New 不应因依赖未就绪返回错误: %v", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("New 阻塞了 %s，期望立即返回", elapsed)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if err := pool.WaitReady(ctx); err == nil {
		t.Fatal("不可达数据库的 WaitReady 应超时")
	}
	checkErr := pool.Check(context.Background())
	if checkErr == nil {
		t.Fatal("Check 应报依赖不可用")
	}
	if strings.Contains(checkErr.Error(), "s3cret") {
		t.Fatalf("错误泄露了口令: %v", checkErr)
	}
	pool.Close()
}

func TestNewWithRealPostgres(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("未执行（环境：无 TEST_DATABASE_URL）")
	}
	pool, err := New(context.Background(), Config{URL: dsn, MaxConns: 2})
	if err != nil {
		t.Fatalf("New 失败: %v", err)
	}
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := pool.WaitReady(ctx); err != nil {
		t.Fatalf("WaitReady 失败: %v", err)
	}
	if err := pool.Check(ctx); err != nil {
		t.Fatalf("Check 失败: %v", err)
	}
	var one int
	if err := pool.Raw().QueryRow(ctx, "SELECT 1").Scan(&one); err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if one != 1 {
		t.Fatalf("SELECT 1 返回 %d", one)
	}
}
