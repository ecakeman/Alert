package consumer

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	portoutbox "github.com/ecakeman/alert/services/service-registry/internal/application/port/outbox"
	portService "github.com/ecakeman/alert/services/service-registry/internal/application/port/service"
	portshared "github.com/ecakeman/alert/services/service-registry/internal/application/port/shared"
	"github.com/ecakeman/alert/services/service-registry/internal/contracts"
)

const consumerName = "registry.monitor-events.v1"

// Deps 是 monitor 事件回写的依赖。
type Deps struct {
	UOW     portshared.UnitOfWork
	Targets portService.TargetRepo
	Sources portService.SourceStateRepo
	Idem    portoutbox.ConsumerIdempotencyRepo
	Clock   portshared.Clock
}

type Consumer struct{ deps Deps }

func New(deps Deps) *Consumer { return &Consumer{deps: deps} }

type targetStatePayload struct {
	ServiceID            string `json:"service_id"`
	TargetID             string `json:"target_id"`
	Status               string `json:"status"`
	ConsecutiveFailures  int    `json:"consecutive_failures"`
	ConsecutiveSuccesses int    `json:"consecutive_successes"`
	LastCheckAt          string `json:"last_check_at"`
}

type serviceStatePayload struct {
	ServiceID          string `json:"service_id"`
	SourceStatus       string `json:"source_status"`
	Status             string `json:"status"`
	AggregationMode    string `json:"aggregation_mode"`
	TargetHealthDigest string `json:"target_health_digest"`
}

// Handle 处理 monitor 事件；未知类型显式失败（消费者不提交）。
func (c *Consumer) Handle(ctx context.Context, body []byte) error {
	var event contracts.Event
	if err := json.Unmarshal(body, &event); err != nil {
		return fmt.Errorf("decode event: %w", err)
	}
	switch event.EventType {
	case contracts.EventMonitorTargetStateChanged:
		return c.handleTargetState(ctx, event)
	case contracts.EventMonitorServiceStateChanged:
		return c.handleServiceState(ctx, event)
	default:
		return fmt.Errorf("unsupported event type %q", event.EventType)
	}
}

func (c *Consumer) handleTargetState(ctx context.Context, event contracts.Event) error {
	var payload targetStatePayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("decode target state payload: %w", err)
	}
	if payload.TargetID == "" || payload.Status == "" {
		return fmt.Errorf("invalid target state payload")
	}
	return c.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		key := "event:" + event.EventID
		eventID := event.EventID
		reserved, err := c.deps.Idem.Reserve(ctx, tx, portoutbox.ConsumerIdempotency{
			Consumer: consumerName, IdempotencyKey: key, EventID: &eventID,
		})
		if err != nil {
			return err
		}
		if !reserved {
			return nil
		}
		lastCheckAt := c.deps.Clock.Now().UTC()
		if payload.LastCheckAt != "" {
			if parsed, err := time.Parse(time.RFC3339, payload.LastCheckAt); err == nil {
				lastCheckAt = parsed
			}
		}
		if err := c.deps.Targets.UpdateProbeResult(ctx, tx, payload.TargetID, payload.Status,
			payload.ConsecutiveFailures, payload.ConsecutiveSuccesses, lastCheckAt); err != nil {
			return err
		}
		return c.deps.Idem.MarkDone(ctx, tx, consumerName, key, "OK", c.deps.Clock.Now().UTC())
	})
}

func (c *Consumer) handleServiceState(ctx context.Context, event contracts.Event) error {
	var payload serviceStatePayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("decode service state payload: %w", err)
	}
	if payload.ServiceID == "" || payload.Status == "" {
		return fmt.Errorf("invalid service state payload")
	}
	return c.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		key := "event:" + event.EventID
		eventID := event.EventID
		reserved, err := c.deps.Idem.Reserve(ctx, tx, portoutbox.ConsumerIdempotency{
			Consumer: consumerName, IdempotencyKey: key, EventID: &eventID,
		})
		if err != nil {
			return err
		}
		if !reserved {
			return nil
		}
		if err := c.deps.Sources.Upsert(ctx, tx, portService.SourceState{
			ServiceID: payload.ServiceID, SourceType: "MONITOR", Status: payload.Status,
		}); err != nil {
			return err
		}
		return c.deps.Idem.MarkDone(ctx, tx, consumerName, key, "OK", c.deps.Clock.Now().UTC())
	})
}
