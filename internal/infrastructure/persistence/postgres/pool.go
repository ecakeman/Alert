package postgres

import (
	"context"
	"fmt"
	"net/url"


	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ecakeman/alert/internal/config"
)

// NewPool 是唯一的 pgxpool 装配入口：解析 DSN、落地池参数、带超时 Ping。
// 错误文本只含主机名，不泄露 DSN（用户名/口令/查询参数）。
func NewPool(ctx context.Context, cfg config.Config) (*pgxpool.Pool, error) {
	pc, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("database unavailable (host=%s)", safeHost(cfg.DatabaseURL))
	}
	pc.MaxConns = int32(cfg.DBMaxConns)
	pc.MinConns = int32(cfg.DBMinConns)
	pc.MaxConnLifetime = cfg.DBMaxConnLifetime
	pc.MaxConnIdleTime = cfg.DBMaxConnIdleTime
	pc.HealthCheckPeriod = cfg.DBHealthcheckPeriod
	pc.ConnConfig.ConnectTimeout = cfg.DBConnectTimeout

	pool, err := pgxpool.NewWithConfig(ctx, pc)
	if err != nil {
		return nil, fmt.Errorf("database unavailable (host=%s)", safeHost(cfg.DatabaseURL))
	}

	pingCtx, cancel := context.WithTimeout(ctx, cfg.DBConnectTimeout)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("database unavailable (host=%s)", safeHost(cfg.DatabaseURL))
	}
	return pool, nil
}

// safeHost 提取 DSN 的 host:port（仅用于错误与日志），解析失败返回 unknown。
func safeHost(databaseURL string) string {
	u, err := url.Parse(databaseURL)
	if err != nil || u.Host == "" {
		return "unknown"
	}
	return u.Host
}
