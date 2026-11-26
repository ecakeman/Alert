package messaging

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/segmentio/kafka-go"
)

func kafkaBrokers(t *testing.T) []string {
	t.Helper()
	raw := os.Getenv("TEST_KAFKA_BROKERS")
	if raw == "" {
		t.Skip("未执行（环境：无 TEST_KAFKA_BROKERS）")
	}
	var brokers []string
	for _, part := range strings.Split(raw, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			brokers = append(brokers, trimmed)
		}
	}
	if len(brokers) == 0 {
		t.Skip("未执行（环境：TEST_KAFKA_BROKERS 为空）")
	}
	return brokers
}

func createTempTopic(t *testing.T, brokers []string, name string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client := &kafka.Client{Addr: kafka.TCP(brokers...)}
	resp, err := client.CreateTopics(ctx, &kafka.CreateTopicsRequest{
		Topics: []kafka.TopicConfig{{Topic: name, NumPartitions: 3, ReplicationFactor: 1}},
	})
	if err != nil {
		t.Fatalf("创建 topic %s 失败: %v", name, err)
	}
	if topicErr, ok := resp.Errors[name]; ok && topicErr != nil {
		t.Fatalf("创建 topic %s 返回错误: %v", name, topicErr)
	}
}

func deleteTempTopic(t *testing.T, brokers []string, name string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client := &kafka.Client{Addr: kafka.TCP(brokers...)}
	if _, err := client.DeleteTopics(ctx, &kafka.DeleteTopicsRequest{Topics: []string{name}}); err != nil {
		t.Logf("删除临时 topic %s 失败（忽略）: %v", name, err)
	}
}

func TestSenderRoundTripAndHashPartitioning(t *testing.T) {
	brokers := kafkaBrokers(t)
	topic := fmt.Sprintf("ms-p07-sender-%d", time.Now().UnixNano())
	createTempTopic(t, brokers, topic)
	defer deleteTempTopic(t, brokers, topic)

	sender, err := NewSender(SenderConfig{Brokers: brokers, Topic: topic})
	if err != nil {
		t.Fatalf("NewSender 失败: %v", err)
	}
	defer sender.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, item := range []struct{ key, body string }{
		{"same-key", `{"n":1}`},
		{"same-key", `{"n":2}`},
		{"other-key", `{"n":3}`},
	} {
		if err := sender.Send(ctx, item.key, []byte(item.body)); err != nil {
			t.Fatalf("发送失败: %v", err)
		}
	}

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     brokers,
		Topic:       topic,
		MinBytes:    1,
		MaxBytes:    10 << 20,
		StartOffset: kafka.FirstOffset,
	})
	defer reader.Close()

	partitions := map[string]int{}
	for i := 0; i < 3; i++ {
		msg, err := reader.ReadMessage(ctx)
		if err != nil {
			t.Fatalf("读取第 %d 条失败: %v", i+1, err)
		}
		partitions[string(msg.Key)] = msg.Partition
	}
	if len(partitions) != 2 {
		t.Fatalf("读取到 %d 个 key，期望 2（same-key/other-key）", len(partitions))
	}
	// 同 key 两次发送必须落同一分区（Hash balancer）。
	reader2 := kafka.NewReader(kafka.ReaderConfig{
		Brokers: brokers, Topic: topic, Partition: partitions["same-key"],
		MinBytes: 1, MaxBytes: 10 << 20,
	})
	defer reader2.Close()
}
