package bffhttp

import (
	"context"
	"sync"
	"time"

	"github.com/ecakeman/alert/services/platform/httpx"
)

// DownstreamChecker 对某个域 /healthz 做探活，结果缓存 10 秒，避免 readyz 放大下游压力。
type DownstreamChecker struct {
	Name     string
	Client   *httpx.Client
	CacheTTL time.Duration
	Now      func() time.Time

	mu      sync.Mutex
	lastAt  time.Time
	lastErr error
}

func (c *DownstreamChecker) Check(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now
	if c.Now != nil {
		now = c.Now
	}
	at := now()
	if !c.lastAt.IsZero() && at.Sub(c.lastAt) < c.CacheTTL {
		return c.lastErr
	}
	err := c.Client.GetJSON(ctx, "/healthz", nil)
	c.lastAt = at
	c.lastErr = err
	return err
}