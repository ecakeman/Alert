package consumer

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ecakeman/alert/services/contracts"

	"github.com/ecakeman/alert/services/episode-svc/internal/application/episode"
	portoutbox "github.com/ecakeman/alert/services/episode-svc/internal/application/port/outbox"
	portshared "github.com/ecakeman/alert/services/episode-svc/internal/application/port/shared"
)

const consumerName = "episode.monitor-events.v1"

type Deps struct {
	UOW      portshared.UnitOfWork
	Idem     portoutbox.ConsumerIdempotencyRepo
	Episodes *episode.Service
	Clock    portshared.Clock
}

type Consumer struct{ deps Deps }

func New(deps Deps) *Consumer { return &Consumer{deps: deps} }

type monitorPayload struct {
	ServiceID          string `json:"service_id"`
	SourceStatus       string `json:"source_status"`
	Status             string `json:"status"`
	AggregationMode    string `json:"aggregation_mode"`
	TargetHealthDigest string `json:"target_health_digest"`
}

// Handle 消费 monitor 服务状态事件：ABNORMAL 立案、NORMAL 尝试解除。
// 已知但与本消费者无关的类型直接提交；未知类型显式失败（不 MarkDone）。
func (c *Consumer) Handle(ctx context.Context, body []byte) error {
	var event contracts.Event
	if err := json.Unmarshal(body, &event); err != nil {
		return fmt.Errorf("decode event: %w", err)
	}
	if !contracts.Supported(event.EventType, event.SchemaVersion) {
		return fmt.Errorf("unsupported event type %q", event.EventType)
	}
	if event.EventType != contracts.EventMonitorServiceStateChanged {
		return nil
	}
	return c.handleMonitor(ctx, event)
}

func (c *Consumer) handleMonitor(ctx context.Context, event contracts.Event) error {
	var payload monitorPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("decode monitor payload: %w", err)
	}
	if payload.ServiceID == "" || payload.Status == "" {
		return fmt.Errorf("invalid monitor payload: missing service_id or status")
	}

	reserved, err := c.reserve(ctx, event.EventID, payload)
	if err != nil {
		return err
	}
	if !reserved {
		return nil
	}

	code := "SKIPPED"
	switch payload.Status {
	case "ABNORMAL":
		if _, _, err := c.deps.Episodes.OpenOrReuse(ctx, payload.ServiceID); err != nil {
			return err
		}
		code = "OPENED"
	case "NORMAL":
		active, err := c.deps.Episodes.ActiveByServices(ctx, []string{payload.ServiceID})
		if err != nil {
			return err
		}
		episodeID := active[payload.ServiceID]
		if episodeID == "" {
			code = "NO_ACTIVE"
			break
		}
		resolved, _, err := c.deps.Episodes.TryResolve(ctx, episodeID)
		if err != nil {
			return err
		}
		if resolved {
			code = "RESOLVED"
		} else {
			code = "NOT_RESOLVED"
		}
	}
	return c.markDone(ctx, event.EventID, code)
}

// reserve 双键幂等：event:<event_id> 与 service:<id>:<status>:<event_id>，任一已存在即视为重放。
func (c *Consumer) reserve(ctx context.Context, eventID string, payload monitorPayload) (bool, error) {
	var ok bool
	err := c.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		id := eventID
		r1, err := c.deps.Idem.Reserve(ctx, tx, portoutbox.ConsumerIdempotency{
			Consumer: consumerName, IdempotencyKey: "event:" + eventID, EventID: &id,
		})
		if err != nil {
			return err
		}
		key2 := fmt.Sprintf("service:%s:%s:%s", payload.ServiceID, payload.Status, eventID)
		r2, err := c.deps.Idem.Reserve(ctx, tx, portoutbox.ConsumerIdempotency{
			Consumer: consumerName, IdempotencyKey: key2, EventID: &id,
		})
		if err != nil {
			return err
		}
		ok = r1 && r2
		return nil
	})
	return ok, err
}

func (c *Consumer) markDone(ctx context.Context, eventID, code string) error {
	return c.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		return c.deps.Idem.MarkDone(ctx, tx, consumerName, "event:"+eventID, code, c.deps.Clock.Now().UTC())
	})
}