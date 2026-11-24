package outbox

import (
	"context"

	"github.com/ecakeman/alert/internal/application/port"
	"github.com/ecakeman/alert/internal/contracts"
)

const auditConsumer = "system.audit.v1"

// Deps 是幂等消费用例依赖。
type ConsumerDeps struct {
	UOW   port.UnitOfWork
	Idem  port.ConsumerIdempotencyRepo
	Clock port.Clock
}

// Consumer 承担“解码后校验 → 幂等预留 → 业务处理 → 完成标记”；
// consumer group 固定 system.audit.v1，幂等键 event_id，完成码 OK。
type Consumer struct{ deps ConsumerDeps }

func NewConsumer(deps ConsumerDeps) *Consumer { return &Consumer{deps: deps} }

// Handle 处理一条已解码事件：
//   - 未知类型/版本 → 返回 ErrUnsupportedEvent，不 MarkDone；
//   - 校验失败 → 返回错误，不 MarkDone；
//   - 幂等预留未命中（重复消息）→ 无副作用返回 nil；
//   - 预留成功 → MarkDone(OK)（短事务）。
func (c *Consumer) Handle(ctx context.Context, ev contracts.Event) error {
	if err := ev.Validate(); err != nil {
		return err
	}

	var reserved bool
	if err := c.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx port.Tx) error {
		var err error
		eventID := ev.EventID
		reserved, err = c.deps.Idem.Reserve(ctx, tx, port.ConsumerIdempotency{
			Consumer:       auditConsumer,
			IdempotencyKey: eventID,
			EventID:        &eventID,
		})
		return err
	}); err != nil {
		return err
	}
	if !reserved {
		return nil
	}

	return c.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx port.Tx) error {
		return c.deps.Idem.MarkDone(ctx, tx, auditConsumer, ev.EventID, "OK", c.deps.Clock.Now())
	})
}
