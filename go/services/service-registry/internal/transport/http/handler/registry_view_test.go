package handler

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	appservice "github.com/ecakeman/alert/services/service-registry/internal/application/service"
	portService "github.com/ecakeman/alert/services/service-registry/internal/application/port/service"
)

func viewTarget(id string, lastCheck *time.Time) portService.MonitorTarget {
	return portService.MonitorTarget{TargetID: id, ServiceID: "svc-1", TargetKey: "k-" + id, LastCheckAt: lastCheck}
}

func marshalKeys(t *testing.T, view appservice.ServiceView) (map[string]json.RawMessage, map[string]any) {
	t.Helper()
	out, err := (&Registry{}).toView(context.Background(), view, false)
	if err != nil {
		t.Fatalf("toView 失败: %v", err)
	}
	body, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal 失败: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("unmarshal 失败: %v", err)
	}
	var generic map[string]any
	_ = json.Unmarshal(body, &generic)
	return raw, generic
}

func TestToViewEmptyTargetsOmitsBothKeys(t *testing.T) {
	raw, _ := marshalKeys(t, appservice.ServiceView{Service: portService.Service{ServiceID: "svc-1"}})
	if _, ok := raw["target_count"]; ok {
		t.Fatal("空 targets 不应出现 target_count")
	}
	if _, ok := raw["last_check_at"]; ok {
		t.Fatal("空 targets 不应出现 last_check_at")
	}
}

func TestToViewTargetsWithoutCheckOmitsLastCheck(t *testing.T) {
	raw, generic := marshalKeys(t, appservice.ServiceView{
		Service: portService.Service{ServiceID: "svc-1"},
		Targets: []portService.MonitorTarget{viewTarget("t1", nil), viewTarget("t2", nil)},
	})
	if _, ok := raw["last_check_at"]; ok {
		t.Fatal("last_check_at 全为 null 时应省略键")
	}
	if int(generic["target_count"].(float64)) != 2 {
		t.Fatalf("target_count = %v，期望 2", generic["target_count"])
	}
}

func TestToViewTargetsWithCheckExposesBothKeys(t *testing.T) {
	earlier := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	later := earlier.Add(2 * time.Hour)
	raw, generic := marshalKeys(t, appservice.ServiceView{
		Service: portService.Service{ServiceID: "svc-1"},
		Targets: []portService.MonitorTarget{
			viewTarget("t1", &earlier),
			viewTarget("t2", &later),
		},
	})
	if int(generic["target_count"].(float64)) != 2 {
		t.Fatalf("target_count = %v，期望 2", generic["target_count"])
	}
	var got string
	if err := json.Unmarshal(raw["last_check_at"], &got); err != nil {
		t.Fatalf("last_check_at 解析失败: %v", err)
	}
	if got != later.Format(time.RFC3339) {
		t.Fatalf("last_check_at = %s，期望 %s", got, later.Format(time.RFC3339))
	}
	if _, ok := raw["config_generation"]; ok {
		t.Fatal("ServiceView 不应暴露 config_generation")
	}
}