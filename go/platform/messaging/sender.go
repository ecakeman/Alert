package messaging

import (
	"context"
	"fmt"
	"time"

	"github.com/segmentio/kafka-go"
)

// SenderConfig 是单域生产者的配置：每域只认一个 PublishTopic。
type SenderConfig struct {
	Brokers      []string
	Topic        string
	BatchTimeout time.Duration
}

// Sender 用 Hash 分区发布原始消息；key 由调用方传入（通常为 aggregate_id）。
// 编解码由服务层注入（platform 不 import contracts）。
type Sender struct {
	writer *kafka.Writer
}

// NewSender 校验配置并构建 writer；topic 显式存在，禁止自动创建。
func NewSender(cfg SenderConfig) (*Sender, error) {
	if len(cfg.Brokers) == 0 {
		return nil, fmt.Errorf("messaging sender: brokers required")
	}
	if cfg.Topic == "" {
		return nil, fmt.Errorf("messaging sender: topic required")
	}
	if cfg.BatchTimeout <= 0 {
		cfg.BatchTimeout = 50 * time.Millisecond
	}
	return &Sender{writer: &kafka.Writer{
		Addr:                   kafka.TCP(cfg.Brokers...),
		Topic:                  cfg.Topic,
		Balancer:               &kafka.Hash{},
		AllowAutoTopicCreation: false,
		RequiredAcks:           kafka.RequireAll,
		BatchTimeout:           cfg.BatchTimeout,
	}}, nil
}

// Topic 返回固定发布 topic。
func (s *Sender) Topic() string { return s.writer.Topic }

// Send 发布一条消息；key 为空时不做分区约束。
func (s *Sender) Send(ctx context.Context, key string, value []byte) error {
	msg := kafka.Message{Value: value}
	if key != "" {
		msg.Key = []byte(key)
	}
	return s.writer.WriteMessages(ctx, msg)
}

// Close 关闭 writer。
func (s *Sender) Close() error { return s.writer.Close() }
