package shared

import (
	"context"
	"time"
)

// Tx 是事务句柄的抽象；由 infrastructure 传入具体实现，application 不感知 pgx。
type Tx any

// UnitOfWork：一次用例一个事务；fn 内的仓储调用共享同一 Tx。
type UnitOfWork interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context, tx Tx) error) error
}

// Clock：UTC 时间源，测试可注入固定时钟。
type Clock interface {
	Now() time.Time
}

// IDGen：业务 ID 生成（UUIDv4 字符串），不依赖 DB 默认值。
type IDGen interface {
	NewID() string
}

// Page 是分页元信息（与条目类型解耦）。
type Page struct {
	Total  int
	Limit  int
	Offset int
}