package consumer

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ecakeman/alert/services/contracts"

	"github.com/ecakeman/alert/services/task-orchestrator/internal/application/apperr"
	"github.com/ecakeman/alert/services/task-orchestrator/internal/application/task"
	"github.com/ecakeman/alert/services/task-orchestrator/internal/domain"
)

// Consumer 订阅 episode.dispatchable 与 task.dispatch.requested 两类事件。
type Consumer struct {
	svc *task.Service
}

func New(svc *task.Service) *Consumer { return &Consumer{svc: svc} }

// Handle 未知类型显式失败（不 MarkDone）；已知但无关的类型直接提交。
func (c *Consumer) Handle(ctx context.Context, body []byte) error {
	var event contracts.Event
	if err := json.Unmarshal(body, &event); err != nil {
		return fmt.Errorf("decode event: %w", err)
	}
	if !contracts.Supported(event.EventType, event.SchemaVersion) {
		return fmt.Errorf("unsupported event type %q", event.EventType)
	}
	switch event.EventType {
	case contracts.EventEpisodeDispatchable:
		err := c.svc.HandleDispatchableEvent(ctx, event)
		// 业务已判定（冲突/重复）不应重试，否则毒消息会阻塞分区。
		if code := apperr.Code(err); code == domain.CodeStateConflict || code == apperr.CodeAlreadyExists {
			return nil
		}
		return err
	case contracts.EventTaskDispatchRequested:
		return c.svc.HandleDispatchEvent(ctx, event)
	default:
		return nil
	}
}