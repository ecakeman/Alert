package messaging

import (
	"context"
	"time"

	"github.com/segmentio/kafka-go"

	"github.com/ecakeman/alert/internal/application/port"
)

// Sender 实现 port.EventSender，用纯 Go 的 kafka-go Writer 发送事件。
type Sender struct {
	writer *kafka.Writer
}

func NewSender(brokers []string, topic string) *Sender {
	return &Sender{writer: &kafka.Writer{
		Addr:                   kafka.TCP(brokers...),
		Topic:                  topic,
		Balancer:               &kafka.Hash{},
		AllowAutoTopicCreation: true,
		RequiredAcks:           kafka.RequireAll,
		BatchTimeout:           50 * time.Millisecond,
	}}
}

func (s *Sender) Send(ctx context.Context, e port.OutboxEvent) error {
	body, err := Encode(Event{
		EventID: e.EventID, EventType: e.EventType, SchemaVersion: e.SchemaVersion,
		AggregateType: e.AggregateType, AggregateID: e.AggregateID,
		OccurredAt: e.CreatedAt, Payload: e.Payload,
	})
	if err != nil {
		return err
	}
	return s.writer.WriteMessages(ctx, kafka.Message{
		Key:   []byte(e.AggregateID),
		Value: body,
	})
}

func (s *Sender) Close() error { return s.writer.Close() }
