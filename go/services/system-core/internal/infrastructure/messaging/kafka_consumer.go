package messaging

import (
	"context"
	"encoding/json"
	"log"

	"github.com/segmentio/kafka-go"
)

// MessageHandler 处理单条消息；返回错误时由调用方决定重试策略。
type MessageHandler func(ctx context.Context, body []byte) error

// Consumer 用 consumer group 消费 topic。
type Consumer struct {
	reader  *kafka.Reader
	handler MessageHandler
	onError func(error)
}

func NewConsumer(brokers []string, topic, groupID string, handler MessageHandler) *Consumer {
	cfg := kafka.ReaderConfig{
		Brokers:        brokers,
		Topic:          topic,
		MinBytes:       1,
		MaxBytes:       10 << 20,
		StartOffset:    kafka.FirstOffset,
		CommitInterval: 0, // 手动提交：处理失败不得提交该条（design 2.5.2 幂等矩阵）
	}
	if groupID != "" {
		cfg.GroupID = groupID
	}
	return &Consumer{reader: kafka.NewReader(cfg), handler: handler}
}

// OnError 注册单条消息处理失败回调；未设置时仅记录日志。
// 单条失败不终止消费，只有 ctx 取消或 reader 关闭才退出。
func (c *Consumer) OnError(fn func(error)) { c.onError = fn }

// Run 持续消费直到 ctx 取消。
// 单条处理失败只记录、不提交该条 offset；成功才 CommitMessages（重启可重放）。
func (c *Consumer) Run(ctx context.Context) error {
	for {
		m, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		if err := c.handler(ctx, m.Value); err != nil {
			c.reportHandlerError(m.Value, err)
			continue
		}
		if err := c.reader.CommitMessages(ctx, m); err != nil && ctx.Err() == nil {
			return err
		}
	}
}

func (c *Consumer) reportHandlerError(body []byte, err error) {
	var meta struct {
		EventID string `json:"event_id"`
	}
	_ = json.Unmarshal(body, &meta)
	log.Printf("ERROR consumer handler failed event_id=%s reason=%v", meta.EventID, err)
	if c.onError != nil {
		c.onError(err)
	}
}

func (c *Consumer) Close() error { return c.reader.Close() }
