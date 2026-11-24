package contracts

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestEventValidateAndSupported(t *testing.T) {
	now := time.Now().UTC()
	ok := Event{
		EventID: "e1", EventType: EventServiceStateChanged, SchemaVersion: 1,
		AggregateType: "service", AggregateID: "s1",
		OccurredAt: now, Payload: json.RawMessage(`{"service_id":"s1"}`),
	}
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid event should pass: %v", err)
	}
	if !Supported(ok.EventType, ok.SchemaVersion) {
		t.Fatal("should be supported")
	}

	unknown := ok
	unknown.EventType = "unknown.v1"
	if err := unknown.Validate(); err == nil {
		t.Fatal("unknown type should fail")
	}
	badVersion := ok
	badVersion.SchemaVersion = 2
	if err := badVersion.Validate(); err == nil {
		t.Fatal("unknown version should fail")
	}

	body, err := json.Marshal(ok)
	if err != nil {
		t.Fatal(err)
	}
	var back Event
	if err := json.Unmarshal(body, &back); err != nil {
		t.Fatal(err)
	}
	if back.EventID != ok.EventID || back.EventType != ok.EventType || string(back.Payload) != string(ok.Payload) {
		t.Fatalf("roundtrip mismatch: %+v", back)
	}
}
func TestEventValidateUnknownVersionReturnsSentinel(t *testing.T) {
	e := Event{
		EventID: "e2", EventType: EventEpisodeInputAppended, SchemaVersion: 99,
		AggregateType: "episode", AggregateID: "ep1",
		OccurredAt: time.Now().UTC(), Payload: json.RawMessage(`{"a":1}`),
	}
	err := e.Validate()
	if !errors.Is(err, ErrUnsupportedEvent) {
		t.Fatalf("expected ErrUnsupportedEvent, got %v", err)
	}

	known := Event{
		EventID: "e3", EventType: EventEpisodeStatusChanged, SchemaVersion: 1,
		AggregateType: "episode", AggregateID: "ep1",
		OccurredAt: time.Now().UTC(), Payload: json.RawMessage(`{"b":2}`),
	}
	if err := known.Validate(); err != nil {
		t.Fatalf("known version should pass: %v", err)
	}
}
