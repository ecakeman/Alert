package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/segmentio/kafka-go"
)

// Handler 处理单条消息；返回错误表示该条不提交（由业务幂等/重试补偿）。
type Handler func(ctx context.Context, body []byte) error

// ConsumerConfig 是消费组配置：多 topic 订阅 + 每域 group。
type ConsumerConfig struct {
	Brokers  []string
	Topics   []string
	GroupID  string
	MaxBytes int
	Logger   *slog.Logger
}

// Consumer 以 consumer group 消费多个 topic；失败消息不提交、继续消费后续消息。
type Consumer struct {
	reader  *kafka.Reader
	handler Handler
	logger  *slog.Logger
}

// NewConsumer 校验配置并构建 reader；StartOffset 固定 FirstOffset。
func NewConsumer(cfg ConsumerConfig, handler Handler) (*Consumer, error) {
	if len(cfg.Brokers) == 0 {
		return nil, fmt.Errorf("messaging consumer: brokers required")
	}
	if len(cfg.Topics) == 0 {
		return nil, fmt.Errorf("messaging consumer: topics required")
	}
	if cfg.GroupID == "" {
		return nil, fmt.Errorf("messaging consumer: group id required")
	}
	if handler == nil {
		return nil, fmt.Errorf("messaging consumer: handler required")
	}
	if cfg.MaxBytes <= 0 {
		cfg.MaxBytes = 10 << 20
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     cfg.Brokers,
		GroupTopics: cfg.Topics,
		GroupID:     cfg.GroupID,
		MinBytes:    1,
		MaxBytes:    cfg.MaxBytes,
		StartOffset: kafka.FirstOffset,
	})
	return &Consumer{reader: reader, handler: handler, logger: logger}, nil
}

// Run 持续消费直到 ctx 取消；单条失败不阻塞后续消息。
// 使用 FetchMessage + CommitMessages 手动提交，确保处理失败的消息不被提交。
func (c *Consumer) Run(ctx context.Context) error {
	for {
		msg, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		if handleErr := c.handler(ctx, msg.Value); handleErr != nil {
			c.report(msg, handleErr)
			continue
		}
		if err := c.reader.CommitMessages(ctx, msg); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
	}
}

func (c *Consumer) report(msg kafka.Message, err error) {
	var meta struct {
		EventID string `json:"event_id"`
	}
	_ = json.Unmarshal(msg.Value, &meta)
	c.logger.Error("messaging consumer handler failed",
		"event_id", meta.EventID,
		"topic", msg.Topic,
		"partition", msg.Partition,
		"offset", msg.Offset,
		"reason", err.Error(),
	)
}

// Close 关闭 reader。
func (c *Consumer) Close() error { return c.reader.Close() }
