package contracts

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestSupportedTypes(t *testing.T) {
	types := SupportedTypes()
	if len(types) != 8 {
		t.Fatalf("SupportedTypes() 返回 %d 类，期望 8 类", len(types))
	}

	seen := make(map[string]bool, len(types))
	for _, typ := range types {
		if seen[typ] {
			t.Fatalf("SupportedTypes() 出现重复类型 %q", typ)
		}
		seen[typ] = true
	}

	for _, typ := range types {
		if !Supported(typ, 1) {
			t.Errorf("Supported(%q, 1) = false，期望 true", typ)
		}
		if Supported(typ, 2) {
			t.Errorf("Supported(%q, 2) = true，期望 false", typ)
		}
	}
	if Supported("unknown.event.v1", 1) {
		t.Error("Supported(unknown.event.v1, 1) = true，期望 false")
	}
}

func TestEventJSONEnvelope(t *testing.T) {
	const sample = `{
		"event_id": "11111111-1111-1111-1111-111111111111",
		"event_type": "episode.dispatchable.v1",
		"schema_version": 1,
		"aggregate_type": "episode",
		"aggregate_id": "22222222-2222-2222-2222-222222222222",
		"occurred_at": "2026-11-25T10:00:00Z",
		"trace_id": "0123456789abcdef0123456789abcdef",
		"payload": {"episode_id":"22222222-2222-2222-2222-222222222222"}
	}`

	var event Event
	if err := json.Unmarshal([]byte(sample), &event); err != nil {
		t.Fatalf("反序列化事件信封失败: %v", err)
	}

	if event.EventID != "11111111-1111-1111-1111-111111111111" {
		t.Errorf("event_id = %q", event.EventID)
	}
	if event.EventType != EventEpisodeDispatchable {
		t.Errorf("event_type = %q，期望 %q", event.EventType, EventEpisodeDispatchable)
	}
	if event.SchemaVersion != 1 {
		t.Errorf("schema_version = %d，期望 1", event.SchemaVersion)
	}
	if event.AggregateType != "episode" {
		t.Errorf("aggregate_type = %q", event.AggregateType)
	}
	if event.AggregateID != "22222222-2222-2222-2222-222222222222" {
		t.Errorf("aggregate_id = %q", event.AggregateID)
	}
	if !event.OccurredAt.Equal(time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)) {
		t.Errorf("occurred_at = %s", event.OccurredAt)
	}
	if event.TraceID != "0123456789abcdef0123456789abcdef" {
		t.Errorf("trace_id = %q", event.TraceID)
	}
	if string(event.Payload) != `{"episode_id":"22222222-2222-2222-2222-222222222222"}` {
		t.Errorf("payload = %s", event.Payload)
	}

	out, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("序列化事件信封失败: %v", err)
	}
	var keys map[string]any
	if err := json.Unmarshal(out, &keys); err != nil {
		t.Fatalf("解析序列化结果失败: %v", err)
	}
	for _, key := range []string{
		"event_id", "event_type", "schema_version", "aggregate_type",
		"aggregate_id", "occurred_at", "trace_id", "payload",
	} {
		if _, ok := keys[key]; !ok {
			t.Errorf("序列化结果缺少 JSON tag %q", key)
		}
	}
}

func TestEventValidate(t *testing.T) {
	valid := Event{
		EventID:       "11111111-1111-1111-1111-111111111111",
		EventType:     EventServiceStateChanged,
		SchemaVersion: 1,
		AggregateType: "service",
		AggregateID:   "22222222-2222-2222-2222-222222222222",
		OccurredAt:    time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC),
		Payload:       json.RawMessage(`{"state":"HEALTHY"}`),
	}

	if err := valid.Validate(); err != nil {
		t.Fatalf("合法事件校验失败: %v", err)
	}

	cases := []struct {
		name   string
		mutate func(Event) Event
	}{
		{
			name: "缺少 event_id",
			mutate: func(e Event) Event {
				e.EventID = ""
				return e
			},
		},
		{
			name: "缺少 aggregate_type",
			mutate: func(e Event) Event {
				e.AggregateType = ""
				return e
			},
		},
		{
			name: "schema_version 为 0",
			mutate: func(e Event) Event {
				e.SchemaVersion = 0
				return e
			},
		},
		{
			name: "空 payload",
			mutate: func(e Event) Event {
				e.Payload = nil
				return e
			},
		},
		{
			name: "不支持的类型",
			mutate: func(e Event) Event {
				e.EventType = "unknown.event.v1"
				return e
			},
		},
		{
			name: "不支持的版本",
			mutate: func(e Event) Event {
				e.SchemaVersion = 2
				return e
			},
		},
		{
			name: "occurred_at 为零值",
			mutate: func(e Event) Event {
				e.OccurredAt = time.Time{}
				return e
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.mutate(valid).Validate(); !errors.Is(err, ErrUnsupportedEvent) {
				t.Fatalf("期望 ErrUnsupportedEvent，实际 %v", err)
			}
		})
	}
}
