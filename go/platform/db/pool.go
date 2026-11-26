package db

import (
	"context"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Config 是 pgxpool 装配的输入；语义与 system-core 的 DB_* 环境变量一致。
type Config struct {
	URL               string
	MaxConns          int32
	MinConns          int32
	ConnectTimeout    time.Duration
	MaxConnLifetime   time.Duration
	MaxConnIdleTime   time.Duration
	HealthCheckPeriod time.Duration
	MaxRetryBackoff   time.Duration
}

// LoadError 描述配置错误；只携带变量名与原因，不回显原值。
type LoadError struct {
	Var    string
	Reason string
}

func (e *LoadError) Error() string { return "config " + e.Var + ": " + e.Reason }

func (c Config) withDefaults() Config {
	if c.MaxConns == 0 {
		c.MaxConns = 10
	}
	if c.ConnectTimeout == 0 {
		c.ConnectTimeout = 5 * time.Second
	}
	if c.MaxConnLifetime == 0 {
		c.MaxConnLifetime = 30 * time.Minute
	}
	if c.MaxConnIdleTime == 0 {
		c.MaxConnIdleTime = 5 * time.Minute
	}
	if c.HealthCheckPeriod == 0 {
		c.HealthCheckPeriod = 30 * time.Second
	}
	if c.MaxRetryBackoff == 0 {
		c.MaxRetryBackoff = 30 * time.Second
	}
	return c
}

// Validate 校验配置；非法项 fail-fast 并指明变量名。
func (c Config) Validate() error {
	if c.URL == "" {
		return &LoadError{Var: "DATABASE_URL", Reason: "missing"}
	}
	if c.MaxConns < 1 || c.MaxConns > 100 {
		return &LoadError{Var: "DB_MAX_CONNS", Reason: "out of range"}
	}
	if c.MinConns < 0 || c.MinConns > c.MaxConns {
		return &LoadError{Var: "DB_MIN_CONNS", Reason: "out of range"}
	}
	if c.ConnectTimeout <= 0 {
		return &LoadError{Var: "DB_CONNECT_TIMEOUT", Reason: "must be positive"}
	}
	if c.MaxConnLifetime <= 0 {
		return &LoadError{Var: "DB_MAX_CONN_LIFETIME", Reason: "must be positive"}
	}
	if c.MaxConnIdleTime <= 0 {
		return &LoadError{Var: "DB_MAX_CONN_IDLE_TIME", Reason: "must be positive"}
	}
	if c.HealthCheckPeriod <= 0 {
		return &LoadError{Var: "DB_HEALTHCHECK_PERIOD", Reason: "must be positive"}
	}
	if c.MaxRetryBackoff <= 0 {
		return &LoadError{Var: "DB_RETRY_BACKOFF", Reason: "must be positive"}
	}
	return nil
}

// Pool 是懒建连的 pgxpool 包装：New 不阻塞等待数据库，后台按指数退避重连。
type Pool struct {
	pool    *pgxpool.Pool
	cfg     Config
	cancel  context.CancelFunc
	done    chan struct{}
	mu      sync.RWMutex
	lastErr error
}

// New 装配连接池；配置非法 fail-fast，依赖未就绪不退出（readyz 如实上报）。
func New(ctx context.Context, cfg Config) (*Pool, error) {
	cfg = cfg.withDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	pc, err := pgxpool.ParseConfig(cfg.URL)
	if err != nil {
		return nil, &LoadError{Var: "DATABASE_URL", Reason: "invalid url"}
	}
	pc.MaxConns = cfg.MaxConns
	pc.MinConns = cfg.MinConns
	pc.MaxConnLifetime = cfg.MaxConnLifetime
	pc.MaxConnIdleTime = cfg.MaxConnIdleTime
	pc.HealthCheckPeriod = cfg.HealthCheckPeriod
	pc.ConnConfig.ConnectTimeout = cfg.ConnectTimeout

	raw, err := pgxpool.NewWithConfig(ctx, pc)
	if err != nil {
		return nil, fmt.Errorf("database unavailable (host=%s)", safeHost(cfg.URL))
	}

	reconnectCtx, cancel := context.WithCancel(context.Background())
	p := &Pool{
		pool:    raw,
		cfg:     cfg,
		cancel:  cancel,
		done:    make(chan struct{}),
		lastErr: fmt.Errorf("database unavailable (host=%s)", safeHost(cfg.URL)),
	}
	go p.ensure(reconnectCtx)
	return p, nil
}

// ensure 使用指数退避建立首个可用连接；成功后退出循环，后续由 pgxpool 自愈。
func (p *Pool) ensure(ctx context.Context) {
	defer close(p.done)
	backoff := time.Second
	for {
		attemptCtx, cancel := context.WithTimeout(ctx, p.cfg.ConnectTimeout)
		err := p.pool.Ping(attemptCtx)
		cancel()
		p.mu.Lock()
		p.lastErr = nil
		if err != nil {
			p.lastErr = fmt.Errorf("database unavailable (host=%s)", safeHost(p.cfg.URL))
		}
		p.mu.Unlock()
		if err == nil {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > p.cfg.MaxRetryBackoff {
			backoff = p.cfg.MaxRetryBackoff
		}
	}
}

// Raw 返回底层连接池；调用方不得关闭。
func (p *Pool) Raw() *pgxpool.Pool { return p.pool }

// Check 实现 DBChecker：上次建连失败时直接返回，否则带超时 Ping。
func (p *Pool) Check(ctx context.Context) error {
	p.mu.RLock()
	lastErr := p.lastErr
	p.mu.RUnlock()
	if lastErr != nil {
		return lastErr
	}
	checkCtx, cancel := context.WithTimeout(ctx, p.cfg.ConnectTimeout)
	defer cancel()
	if err := p.pool.Ping(checkCtx); err != nil {
		return fmt.Errorf("database unavailable (host=%s)", safeHost(p.cfg.URL))
	}
	return nil
}

// WaitReady 阻塞等待首个连接成功或 ctx 取消，供启动自检与集成测试使用。
func (p *Pool) WaitReady(ctx context.Context) error {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := p.Check(ctx); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// Close 停止重连并释放连接池。
func (p *Pool) Close() {
	p.cancel()
	<-p.done
	p.pool.Close()
}

// safeHost 提取 DSN 的 host:port（仅用于错误与日志），解析失败返回 unknown。
func safeHost(databaseURL string) string {
	u, err := url.Parse(databaseURL)
	if err != nil || u.Host == "" {
		return "unknown"
	}
	return u.Host
}
