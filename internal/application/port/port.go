package port

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

// EventSender：Kafka 发送端口，事务外调用（B5 实现）。
type EventSender interface {
	Send(ctx context.Context, event OutboxEvent) error
}

// TaskDispatcher：任务提交端口，事务外调用（B7 实现）。
type TaskDispatcher interface {
	SubmitTask(ctx context.Context, task AgentTask) (accepted bool, err error)
}

// Prober：真实探测端口（B6 实现），不写库。
type Prober interface {
	Probe(ctx context.Context, target MonitorTarget) (MonitorPoint, error)
}
