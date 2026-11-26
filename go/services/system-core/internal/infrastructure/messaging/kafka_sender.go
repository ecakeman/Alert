package messaging

import (
	"context"
	"time"

	"github.com/segmentio/kafka-go"

	"github.com/ecakeman/alert/services/system-core/internal/contracts"
)

// Sender 实现 port.EventSender，用纯 Go 的 kafka-go Writer 发送契约事件。
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

func (s *Sender) Send(ctx context.Context, e contracts.Event) error {
	body, err := Encode(e)
	if err != nil {
		return err
	}
	return s.writer.WriteMessages(ctx, kafka.Message{
		Key:   []byte(e.AggregateID),
		Value: body,
	})
}

func (s *Sender) Close() error { return s.writer.Close() }
