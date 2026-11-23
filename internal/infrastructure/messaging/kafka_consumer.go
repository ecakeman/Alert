package messaging

import (
	"context"

	"github.com/segmentio/kafka-go"
)

// MessageHandler 处理单条消息；返回错误时由调用方决定重试策略。
type MessageHandler func(ctx context.Context, body []byte) error

// Consumer 用 consumer group 消费 topic。
type Consumer struct {
	reader  *kafka.Reader
	handler MessageHandler
}

func NewConsumer(brokers []string, topic, groupID string, handler MessageHandler) *Consumer {
	cfg := kafka.ReaderConfig{
		Brokers:     brokers,
		Topic:       topic,
		MinBytes:    1,
		MaxBytes:    10 << 20,
		StartOffset: kafka.FirstOffset,
	}
	if groupID != "" {
		cfg.GroupID = groupID
	}
	return &Consumer{reader: kafka.NewReader(cfg), handler: handler}
}

// Run 持续消费直到 ctx 取消。
func (c *Consumer) Run(ctx context.Context) error {
	for {
		m, err := c.reader.ReadMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		_ = c.handler(ctx, m.Value)
	}
}

func (c *Consumer) Close() error { return c.reader.Close() }