package projection

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/ecakeman/alert/services/monitor-svc/internal/application/port/outbox"
	"github.com/ecakeman/alert/services/monitor-svc/internal/application/port/projection"
	portshared "github.com/ecakeman/alert/services/monitor-svc/internal/application/port/shared"
	"github.com/ecakeman/alert/services/monitor-svc/internal/contracts"
)

const consumerName = "monitor.catalog-events.v1"

// Deps 是 catalog 投影消费的依赖。
type Deps struct {
	UOW        portshared.UnitOfWork
	Projection projection.Repo
	Idem       outbox.ConsumerIdempotencyRepo
	Clock      portshared.Clock
	Synced     *SyncState
}

// SyncState 供 readyz 判断"是否至少消费过一次 catalog"。
type SyncState struct{ done atomic.Bool }

func NewSyncState() *SyncState { return &SyncState{} }

func (s *SyncState) Mark()          { s.done.Store(true) }
func (s *SyncState) Synced() bool   { return s.done.Load() }
func (s *SyncState) Reset()         { s.done.Store(false) }

// Check 实现 httpx.Checker：未同步时 readyz 非 200。
func (s *SyncState) Check(context.Context) error {
	if !s.done.Load() {
		return fmt.Errorf("catalog projection not synced")
	}
	return nil
}

type Consumer struct{ deps Deps }

func New(deps Deps) *Consumer { return &Consumer{deps: deps} }

type catalogService struct {
	ID              string `json:"id"`
	Enabled         bool   `json:"enabled"`
	AggregationMode string `json:"aggregation_mode"`
}

type catalogTarget struct {
	TargetID          string `json:"target_id"`
	TargetKey         string `json:"target_key"`
	TargetType        string `json:"target_type"`
	Address           string `json:"address"`
	IntervalMS        int    `json:"interval_ms"`
	TimeoutMS         int    `json:"timeout_ms"`
	FailureThreshold  int    `json:"failure_threshold"`
	RecoveryThreshold int    `json:"recovery_threshold"`
	IsRequired        bool   `json:"is_required"`
	Enabled           bool   `json:"enabled"`
	ConfigGeneration  int64  `json:"config_generation"`
}

type catalogPayload struct {
	Service catalogService  `json:"service"`
	Targets []catalogTarget `json:"targets"`
}

// Handle 处理 catalog 事件。
// 同一 registry topic 还承载 service.state.changed.v1（registry 内部语义）：
// 该类型对本消费者"已知但无关"，跳过并提交；真正未知的类型显式失败（不 MarkDone）。
func (c *Consumer) Handle(ctx context.Context, body []byte) error {
	var event contracts.Event
	if err := json.Unmarshal(body, &event); err != nil {
		return fmt.Errorf("decode event: %w", err)
	}
	if !contracts.Supported(event.EventType, event.SchemaVersion) {
		return fmt.Errorf("unsupported event type %q", event.EventType)
	}
	if event.EventType != contracts.EventServiceCatalogChanged {
		return nil
	}
	return c.handleCatalog(ctx, event)
}

func (c *Consumer) handleCatalog(ctx context.Context, event contracts.Event) error {
	var payload catalogPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("decode catalog payload: %w", err)
	}
	if payload.Service.ID == "" {
		return fmt.Errorf("invalid catalog payload: missing service id")
	}
	now := c.deps.Clock.Now().UTC()
	return c.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		key := "event:" + event.EventID
		eventID := event.EventID
		reserved, err := c.deps.Idem.Reserve(ctx, tx, outbox.ConsumerIdempotency{
			Consumer: consumerName, IdempotencyKey: key, EventID: &eventID,
		})
		if err != nil {
			return err
		}
		if !reserved {
			return nil
		}

		keep := make([]string, 0, len(payload.Targets))
		for _, t := range payload.Targets {
			if err := c.deps.Projection.Upsert(ctx, tx, projection.Target{
				TargetID: t.TargetID, ServiceID: payload.Service.ID, TargetKey: t.TargetKey,
				TargetType: t.TargetType, Address: t.Address, IntervalMS: t.IntervalMS, TimeoutMS: t.TimeoutMS,
				FailureThreshold: t.FailureThreshold, RecoveryThreshold: t.RecoveryThreshold,
				IsRequired: t.IsRequired, Enabled: t.Enabled, ConfigGeneration: t.ConfigGeneration,
				MonitorAggregationMode: payload.Service.AggregationMode,
			}, now); err != nil {
				return err
			}
			keep = append(keep, t.TargetID)
		}
		if !payload.Service.Enabled {
			if err := c.deps.Projection.MarkServiceDeleted(ctx, tx, payload.Service.ID, now); err != nil {
				return err
			}
		} else if err := c.deps.Projection.MarkDeleted(ctx, tx, payload.Service.ID, keep, now); err != nil {
			return err
		}
		if err := c.deps.Idem.MarkDone(ctx, tx, consumerName, key, "OK", now); err != nil {
			return err
		}
		if c.deps.Synced != nil {
			c.deps.Synced.Mark()
		}
		return nil
	})
}

var _ = time.Time{}