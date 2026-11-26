package outbox

import (
	"context"
	"testing"
	"time"

	portoutbox "github.com/ecakeman/alert/services/system-core/internal/application/port/outbox"
	portshared "github.com/ecakeman/alert/services/system-core/internal/application/port/shared"
	"github.com/ecakeman/alert/services/system-core/internal/contracts"
)

type fakeIdem struct {
	reserved map[string]bool
	marked   map[string]string
}

func (f *fakeIdem) Reserve(ctx context.Context, tx portshared.Tx, idem portoutbox.ConsumerIdempotency) (bool, error) {
	if f.reserved[idem.IdempotencyKey] {
		return false, nil
	}
	f.reserved[idem.IdempotencyKey] = true
	return true, nil
}

func (f *fakeIdem) MarkDone(ctx context.Context, tx portshared.Tx, consumer, key, resultCode string, at time.Time) error {
	f.marked[key] = resultCode
	return nil
}

type fakeUOW struct{}

func (fakeUOW) WithinTx(ctx context.Context, fn func(context.Context, portshared.Tx) error) error {
	return fn(ctx, nil)
}

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

// TestConsumerHandle：首次处理标记完成、重复处理无副作用、未知版本返回错误且不标记。
func TestConsumerHandle(t *testing.T) {
	clk := fixedClock{t: time.Now().UTC()}
	idem := &fakeIdem{reserved: map[string]bool{}, marked: map[string]string{}}
	c := NewConsumer(ConsumerDeps{UOW: fakeUOW{}, Idem: idem, Clock: clk})
	ctx := context.Background()

	ev := contracts.Event{
		EventID: "e1", EventType: contracts.EventServiceStateChanged, SchemaVersion: 1,
		AggregateType: "service", AggregateID: "s1",
		OccurredAt: clk.t, Payload: []byte(`{"service_id":"s1"}`),
	}

	if err := c.Handle(ctx, ev); err != nil {
		t.Fatalf("first handle: %v", err)
	}
	if idem.marked[ev.EventID] != "OK" {
		t.Fatalf("first handle should mark done: %+v", idem.marked)
	}

	if err := c.Handle(ctx, ev); err != nil {
		t.Fatalf("duplicate handle: %v", err)
	}
	if len(idem.reserved) != 1 {
		t.Fatalf("duplicate should not reserve again: %+v", idem.reserved)
	}

	unknown := ev
	unknown.EventID = "e2"
	unknown.EventType = "unknown.v1"
	unknown.SchemaVersion = 99
	if err := c.Handle(ctx, unknown); err == nil {
		t.Fatal("unknown version should error")
	}
	if _, ok := idem.marked[unknown.EventID]; ok {
		t.Fatal("unknown version must not be marked done")
	}
}
