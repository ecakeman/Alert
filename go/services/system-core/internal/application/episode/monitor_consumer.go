package episode

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	portepisode "github.com/ecakeman/alert/services/system-core/internal/application/port/episode"
	portoutbox "github.com/ecakeman/alert/services/system-core/internal/application/port/outbox"
	portshared "github.com/ecakeman/alert/services/system-core/internal/application/port/shared"
	"github.com/ecakeman/alert/services/system-core/internal/contracts"
)

// MonitorConsumerName 是单体消费 monitor 状态事件的 consumer group。
const MonitorConsumerName = "episode.monitor-events.v1"

// MonitorConsumerDeps 是事件驱动的立案/解除依赖。
type MonitorConsumerDeps struct {
	UOW      portshared.UnitOfWork
	Episodes portepisode.EpisodeRepo
	Idem     portoutbox.ConsumerIdempotencyRepo
	Clock    portshared.Clock
	Service  *Service
}

// MonitorConsumer 消费 monitor.service.state.changed.v1：
// ABNORMAL → OpenOrReuse；NORMAL → 查 ACTIVE → TryResolve。处理本身幂等，事件重放安全。
type MonitorConsumer struct{ deps MonitorConsumerDeps }

func NewMonitorConsumer(deps MonitorConsumerDeps) *MonitorConsumer {
	return &MonitorConsumer{deps: deps}
}

type monitorStatePayload struct {
	ServiceID string `json:"service_id"`
	Status    string `json:"status"`
}

// Handle 处理 monitor 事件；真正未知的类型显式失败（不提交）。
func (c *MonitorConsumer) Handle(ctx context.Context, body []byte) error {
	var event contracts.Event
	if err := json.Unmarshal(body, &event); err != nil {
		return fmt.Errorf("decode event: %w", err)
	}
	if err := event.Validate(); err != nil {
		return fmt.Errorf("validate event: %w", err)
	}
	if event.EventType != contracts.EventMonitorServiceStateChanged {
		// 已知但无关的类型（同 topic 混投）跳过；Validate 已拒绝真正未知类型。
		return nil
	}

	var payload monitorStatePayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("decode monitor payload: %w", err)
	}
	if payload.ServiceID == "" || payload.Status == "" {
		return fmt.Errorf("invalid monitor payload")
	}

	switch payload.Status {
	case "ABNORMAL":
		if _, _, err := c.deps.Service.OpenOrReuse(ctx, payload.ServiceID); err != nil {
			return err
		}
	case "NORMAL":
		episodeID, err := c.activeEpisode(ctx, payload.ServiceID)
		if err != nil {
			return err
		}
		if episodeID != "" {
			if _, _, err := c.deps.Service.TryResolve(ctx, episodeID); err != nil {
				return err
			}
		}
	default:
		return nil
	}

	// 处理成功后落幂等：event:<id> + monitor:<service>:<status>:<occurred_at> 双键。
	return c.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		eventID := event.EventID
		keys := []string{
			"event:" + event.EventID,
			"monitor:" + payload.ServiceID + ":" + payload.Status + ":" + event.OccurredAt.UTC().Format(time.RFC3339Nano),
		}
		for _, key := range keys {
			reserved, err := c.deps.Idem.Reserve(ctx, tx, portoutbox.ConsumerIdempotency{
				Consumer: MonitorConsumerName, IdempotencyKey: key, EventID: &eventID,
			})
			if err != nil {
				return err
			}
			if !reserved {
				continue
			}
			if err := c.deps.Idem.MarkDone(ctx, tx, MonitorConsumerName, key, "OK", c.deps.Clock.Now().UTC()); err != nil {
				return err
			}
		}
		return nil
	})
}

func (c *MonitorConsumer) activeEpisode(ctx context.Context, serviceID string) (string, error) {
	var episodeID string
	err := c.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		ep, err := c.deps.Episodes.GetActiveByService(ctx, tx, serviceID)
		if err != nil {
			return nil
		}
		episodeID = ep.EpisodeID
		return nil
	})
	return episodeID, err
}
