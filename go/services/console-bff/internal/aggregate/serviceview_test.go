package aggregate

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ecakeman/alert/services/platform/httpx"
)

func keySet(t *testing.T, raw []byte) map[string]bool {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("解析输出失败: %v", err)
	}
	out := make(map[string]bool, len(m))
	for k := range m {
		out[k] = true
	}
	return out
}

func assertKeys(t *testing.T, got, want map[string]bool) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("键数 = %d，期望 %d：got=%v want=%v", len(got), len(want), got, want)
	}
	for k := range want {
		if !got[k] {
			t.Fatalf("缺少键 %q：got=%v", k, got)
		}
	}
}

const targetL2 = `{"target_id":"t1","target_key":"k","target_name":"n","target_type":"HTTP","address":"http://x","config":{},"interval_ms":15000,"timeout_ms":3000,"failure_threshold":2,"recovery_threshold":2,"is_required":true,"enabled":true,"status":"NORMAL","consecutive_failures":0,"consecutive_successes":3,"last_check_at":"2026-11-01T00:00:00Z","config_generation":1}`

func TestProjectTargetTrimsConfigGeneration(t *testing.T) {
	out, err := projectTargetL1([]byte(targetL2))
	if err != nil {
		t.Fatalf("projectTargetL1 失败: %v", err)
	}
	got := keySet(t, out)
	want := map[string]bool{
		"target_id": true, "target_key": true, "target_name": true, "target_type": true,
		"address": true, "config": true, "interval_ms": true, "timeout_ms": true,
		"failure_threshold": true, "recovery_threshold": true, "is_required": true,
		"enabled": true, "status": true, "consecutive_failures": true,
		"consecutive_successes": true, "last_check_at": true,
	}
	assertKeys(t, got, want)
}

func TestProjectTargetsListWraps(t *testing.T) {
	out, err := projectTargetsListL1([]byte("[" + targetL2 + "]"))
	if err != nil {
		t.Fatalf("projectTargetsListL1 失败: %v", err)
	}
	keys := keySet(t, out)
	if len(keys) != 1 || !keys["targets"] {
		t.Fatalf("07 顶层键集合应为 [targets]，实际 %v", keys)
	}
	empty, _ := projectTargetsListL1([]byte("[]"))
	if string(empty) != `{"targets":[]}` {
		t.Fatalf("空列表应为 {\"targets\":[]}，实际 %s", string(empty))
	}
}

func TestProjectServiceViewNoActiveOmitsKeys(t *testing.T) {
	upstream := `{"service_id":"svc-1","service_key":"k","name":"n","description":"d","enabled":true,"monitor_aggregation_mode":"ALL_REQUIRED","authority_producer_id":"p","monitor_status":"UNKNOWN","app_status":"UNKNOWN","targets":[]}`
	out, err := projectServiceViewL1([]byte(upstream), nil)
	if err != nil {
		t.Fatalf("projectServiceViewL1 失败: %v", err)
	}
	keys := keySet(t, out)
	for _, k := range []string{"target_count", "last_check_at", "active_episode_id"} {
		if keys[k] {
			t.Fatalf("03 不应出现 %q", k)
		}
	}
}

func TestProjectL1ActiveCallsEpisodeOnce(t *testing.T) {
	var calls int32
	ep := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/internal/episodes/active-by-services" {
			atomic.AddInt32(&calls, 1)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"svc-1": "ep-1"})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ep.Close()

	svc := New(Clients{Episode: httpx.NewClient(ep.URL, time.Second)}, time.Second, nil)
	upstream := `{"service_id":"svc-1","service_key":"k","name":"n","description":"d","enabled":true,"monitor_aggregation_mode":"ALL_REQUIRED","authority_producer_id":"p","monitor_status":"NORMAL","app_status":"NORMAL","targets":[` + targetL2 + `],"target_count":1,"last_check_at":"2026-11-01T00:00:00Z"}`
	out, err := svc.ProjectL1(context.Background(), L1ServiceViewActive, []byte(upstream))
	if err != nil {
		t.Fatalf("ProjectL1 失败: %v", err)
	}
	var view map[string]any
	_ = json.Unmarshal(out, &view)
	if view["active_episode_id"] != "ep-1" {
		t.Fatalf("active_episode_id = %v，期望 ep-1", view["active_episode_id"])
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("episode 调用次数 = %d，期望 1", calls)
	}
}