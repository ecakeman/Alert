package outbox

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

type fakeTx struct {
	pgx.Tx
	committed int
	rolled    int
}

func (t *fakeTx) Commit(context.Context) error   { t.committed++; return nil }
func (t *fakeTx) Rollback(context.Context) error { t.rolled++; return nil }

type fakeBeginner struct{ txs []*fakeTx }

func (b *fakeBeginner) Begin(context.Context) (pgx.Tx, error) {
	tx := &fakeTx{}
	b.txs = append(b.txs, tx)
	return tx, nil
}

type fakeStore struct {
	records []Record
	acked   []string
	failed  map[string]string
	limit   int
	until   time.Time
}

func (s *fakeStore) Claim(_ context.Context, _ DBTX, _ string, until time.Time, limit int) ([]Record, error) {
	s.limit = limit
	s.until = until
	recs := s.records
	s.records = nil
	return recs, nil
}

func (s *fakeStore) Ack(_ context.Context, _ DBTX, eventID string, _ time.Time) error {
	s.acked = append(s.acked, eventID)
	return nil
}

func (s *fakeStore) Fail(_ context.Context, _ DBTX, eventID string, _ time.Time, msg string) error {
	if s.failed == nil {
		s.failed = map[string]string{}
	}
	s.failed[eventID] = msg
	return nil
}

type fakeSender struct {
	err       error
	topics    []string
	remaining []time.Duration
}

func (s *fakeSender) Send(ctx context.Context, topic string, _ Record) error {
	s.topics = append(s.topics, topic)
	if deadline, ok := ctx.Deadline(); ok {
		s.remaining = append(s.remaining, time.Until(deadline))
	}
	return s.err
}

func newTestPublisher(t *testing.T, store Store, beginner TxBeginner, sender Sender, lease time.Duration) *Publisher {
	t.Helper()
	p, err := New(Deps{
		Store:        store,
		Tx:           beginner,
		Sender:       sender,
		Owner:        "task-orchestrator",
		PublishTopic: "alertflow.task.events.v1",
		ClaimLease:   lease,
		Batch:        4,
		PollInterval: time.Millisecond,
		Now:          func() time.Time { return time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("New 失败: %v", err)
	}
	return p
}

func TestNewRequiresDependencies(t *testing.T) {
	base := Deps{Store: &fakeStore{}, Tx: &fakeBeginner{}, Sender: &fakeSender{}, Owner: "o", PublishTopic: "t"}
	cases := []struct {
		name string
		mut  func(Deps) Deps
	}{
		{"store", func(d Deps) Deps { d.Store = nil; return d }},
		{"tx", func(d Deps) Deps { d.Tx = nil; return d }},
		{"sender", func(d Deps) Deps { d.Sender = nil; return d }},
		{"owner", func(d Deps) Deps { d.Owner = ""; return d }},
		{"topic", func(d Deps) Deps { d.PublishTopic = ""; return d }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := New(tc.mut(base)); err == nil {
				t.Fatalf("缺少 %s 应报错", tc.name)
			}
		})
	}
}

func TestRunOnceClaimsSendsAndAcks(t *testing.T) {
	store := &fakeStore{records: []Record{
		{EventID: "e1", EventType: "task.dispatch.requested.v1"},
		{EventID: "e2", EventType: "task.dispatch.requested.v1"},
	}}
	beginner := &fakeBeginner{}
	sender := &fakeSender{}
	p := newTestPublisher(t, store, beginner, sender, 10*time.Second)

	sent, err := p.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce 失败: %v", err)
	}
	if sent != 2 || len(store.acked) != 2 {
		t.Fatalf("sent=%d acked=%v", sent, store.acked)
	}
	if len(sender.topics) != 2 || sender.topics[0] != "alertflow.task.events.v1" {
		t.Fatalf("topic 注入错误: %v", sender.topics)
	}
	if len(beginner.txs) != 3 {
		t.Fatalf("事务数 = %d，期望 1 claim + 2 ack", len(beginner.txs))
	}
	for i, tx := range beginner.txs {
		if tx.committed != 1 || tx.rolled != 0 {
			t.Fatalf("事务 %d commit=%d rollback=%d", i, tx.committed, tx.rolled)
		}
	}
}

func TestRunOnceUsesHalfLeaseAsSendTimeout(t *testing.T) {
	store := &fakeStore{records: []Record{{EventID: "e1"}}}
	sender := &fakeSender{}
	p := newTestPublisher(t, store, &fakeBeginner{}, sender, 10*time.Second)

	if _, err := p.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce 失败: %v", err)
	}
	if p.SendTimeout() != 5*time.Second {
		t.Fatalf("SendTimeout = %s", p.SendTimeout())
	}
	if len(sender.remaining) != 1 {
		t.Fatalf("未记录发送超时")
	}
	if sender.remaining[0] > 5*time.Second || sender.remaining[0] < 4*time.Second {
		t.Fatalf("发送超时剩余 = %s，期望约 5s", sender.remaining[0])
	}
}

func TestRunOnceMarksFailureWithoutAck(t *testing.T) {
	store := &fakeStore{records: []Record{{EventID: "e1"}, {EventID: "e2"}}}
	sender := &fakeSender{err: errors.New("kafka down")}
	p := newTestPublisher(t, store, &fakeBeginner{}, sender, 10*time.Second)

	sent, err := p.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce 失败: %v", err)
	}
	if sent != 0 || len(store.acked) != 0 {
		t.Fatalf("sent=%d acked=%v", sent, store.acked)
	}
	if len(store.failed) != 2 || store.failed["e1"] != "kafka down" {
		t.Fatalf("failed = %v", store.failed)
	}
}

func TestRunOnceDefaultLeaseAndBatch(t *testing.T) {
	store := &fakeStore{}
	p, err := New(Deps{
		Store: store, Tx: &fakeBeginner{}, Sender: &fakeSender{},
		Owner: "o", PublishTopic: "t",
	})
	if err != nil {
		t.Fatalf("New 失败: %v", err)
	}
	if p.SendTimeout() != 15*time.Second {
		t.Fatalf("默认租约下 SendTimeout = %s，期望 15s", p.SendTimeout())
	}
	if _, err := p.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce 失败: %v", err)
	}
	if store.limit != 16 {
		t.Fatalf("默认 batch = %d，期望 16", store.limit)
	}
}
