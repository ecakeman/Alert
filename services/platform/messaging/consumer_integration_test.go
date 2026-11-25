package messaging

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestConsumerMultiTopicAndHandlerError(t *testing.T) {
	brokers := kafkaBrokers(t)
	prefix := fmt.Sprintf("ms-p07-consumer-%d", time.Now().UnixNano())
	topicA := prefix + "-a"
	topicB := prefix + "-b"
	createTempTopic(t, brokers, topicA)
	createTempTopic(t, brokers, topicB)
	defer deleteTempTopic(t, brokers, topicA)
	defer deleteTempTopic(t, brokers, topicB)

	senderA, err := NewSender(SenderConfig{Brokers: brokers, Topic: topicA})
	if err != nil {
		t.Fatalf("NewSender A 失败: %v", err)
	}
	defer senderA.Close()
	senderB, err := NewSender(SenderConfig{Brokers: brokers, Topic: topicB})
	if err != nil {
		t.Fatalf("NewSender B 失败: %v", err)
	}
	defer senderB.Close()

	received := make(chan string, 4)
	consumer, err := NewConsumer(ConsumerConfig{
		Brokers: brokers,
		Topics:  []string{topicA, topicB},
		GroupID: prefix + "-group",
	}, func(_ context.Context, body []byte) error {
		text := string(body)
		received <- text
		if strings.Contains(text, "fail") {
			return fmt.Errorf("expected handler failure")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("NewConsumer 失败: %v", err)
	}
	defer consumer.Close()

	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- consumer.Run(runCtx) }()

	ctx, cancelSend := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelSend()
	payloads := []string{`{"event_id":"ok-1"}`, `{"event_id":"fail-2"}`, `{"event_id":"ok-3"}`}
	if err := senderA.Send(ctx, "k1", []byte(payloads[0])); err != nil {
		t.Fatalf("发送 A 失败: %v", err)
	}
	if err := senderA.Send(ctx, "k2", []byte(payloads[1])); err != nil {
		t.Fatalf("发送 A2 失败: %v", err)
	}
	if err := senderB.Send(ctx, "k3", []byte(payloads[2])); err != nil {
		t.Fatalf("发送 B 失败: %v", err)
	}

	seen := map[string]bool{}
	timeout := time.After(15 * time.Second)
	for len(seen) < 3 {
		select {
		case body := <-received:
			seen[body] = true
		case <-timeout:
			t.Fatalf("等待消息超时，已收到 %v", seen)
		}
	}
	// 失败消息之后仍继续消费，说明单条失败不阻塞。
	if !seen[payloads[2]] {
		t.Fatalf("失败消息之后的 topic 消息未被消费: %v", seen)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run 退出错误: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run 未在超时内退出")
	}
}
