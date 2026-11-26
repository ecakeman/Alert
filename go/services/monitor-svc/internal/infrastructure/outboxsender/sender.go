package outboxsender

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ecakeman/alert/services/platform/messaging"
	"github.com/ecakeman/alert/services/platform/outbox"

	"github.com/ecakeman/alert/services/monitor-svc/internal/contracts"
)

// Sender 把 Outbox 记录编码为事件信封后写入固定 topic。
type Sender struct {
	kafka *messaging.Sender
}

func New(kafka *messaging.Sender) *Sender { return &Sender{kafka: kafka} }

func (s *Sender) Send(ctx context.Context, topic string, rec outbox.Record) error {
	if topic != s.kafka.Topic() {
		return fmt.Errorf("outbox sender: unexpected topic")
	}
	body, err := json.Marshal(contracts.Event{
		EventID: rec.EventID, EventType: rec.EventType, SchemaVersion: rec.SchemaVersion,
		AggregateType: rec.AggregateType, AggregateID: rec.AggregateID,
		OccurredAt: rec.OccurredAt, TraceID: rec.TraceID, Payload: rec.Payload,
	})
	if err != nil {
		return err
	}
	return s.kafka.Send(ctx, rec.AggregateID, body)
}
