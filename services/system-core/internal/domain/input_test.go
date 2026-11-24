package domain

import (
	"encoding/json"
	"testing"
)

// INV-04：同内容不造新版本（同 hash）、真变化必造新版本；顺序无关。
func TestINV04_FingerprintStable(t *testing.T) {
	base := []MonitorField{
		{TargetKey: "a", TargetType: "HTTP", Status: "NORMAL"},
		{TargetKey: "b", TargetType: "TCP", Status: "ABNORMAL"},
	}
	h1, err := Fingerprint(base, nil)
	if err != nil {
		t.Fatal(err)
	}

	// 字段顺序不同 -> 同 hash
	h2, err := Fingerprint([]MonitorField{base[1], base[0]}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if h1 != h2 {
		t.Fatal("projection order must not change fingerprint")
	}

	// 真实状态变化 -> 不同 hash
	changed := []MonitorField{
		{TargetKey: "a", TargetType: "HTTP", Status: "ABNORMAL"},
		base[1],
	}
	h3, err := Fingerprint(changed, nil)
	if err != nil {
		t.Fatal(err)
	}
	if h1 == h3 {
		t.Fatal("status change must change fingerprint")
	}

	// 应用事件追加顺序属于语义，顺序变化应改变 hash
	e1 := []AppCanon{{EventKind: "HEALTH_STATUS", HealthStatus: "NORMAL"}}
	e2 := []AppCanon{
		{EventKind: "HEALTH_STATUS", HealthStatus: "ABNORMAL"},
		{EventKind: "HEALTH_STATUS", HealthStatus: "NORMAL"},
	}
	he1, _ := Fingerprint(base, e1)
	he2, _ := Fingerprint(base, e2)
	if he1 == he2 {
		t.Fatal("app event append order must affect fingerprint")
	}
}

// INV-05：同键同内容幂等、同键不同内容冲突（不覆盖）、新键追加。
func TestINV05_AppEventDecision(t *testing.T) {
	base := AppEvent{
		ServiceID: "s", ProducerID: "p", EventID: "e",
		EventKind: "HEALTH_STATUS", HealthStatus: "ABNORMAL",
		Payload: json.RawMessage(`{"a":1}`),
	}

	if r := DecideAppEvent(nil, base); r.Decision != AppEventAppend {
		t.Fatalf("nil existing should append, got %v", r.Decision)
	}
	if r := DecideAppEvent(&base, base); r.Decision != AppEventDuplicate {
		t.Fatalf("same identity+content should duplicate, got %v", r.Decision)
	}

	diff := base
	diff.HealthStatus = "NORMAL"
	r := DecideAppEvent(&base, diff)
	if r.Decision != AppEventConflict {
		t.Fatalf("same identity different content should conflict, got %v", r.Decision)
	}
	if !IsCode(r.Err, CodeEventIDConflict) {
		t.Fatalf("conflict must carry EVENT_ID_CONFLICT, got %v", r.Err)
	}

	other := base
	other.EventID = "e2"
	if r := DecideAppEvent(&base, other); r.Decision != AppEventAppend {
		t.Fatalf("different event id should append, got %v", r.Decision)
	}
}