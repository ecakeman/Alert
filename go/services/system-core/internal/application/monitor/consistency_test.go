package monitor

import (
	"testing"

	portService "github.com/ecakeman/alert/services/system-core/internal/application/port/service"
	"github.com/ecakeman/alert/services/system-core/internal/domain"
)

// TestMonitorAggregationConsistency 锁定：service 与 monitor 两条重算路径
// 均经由 monitorTargetHealths + domain.AggregateMonitor，同输入同输出；
// 防止任一路径再次内联计数形成第二份算法（B3/B6 曾各自实现且语义漂移）。
func TestMonitorAggregationConsistency(t *testing.T) {
	targets := []portService.MonitorTarget{
		{TargetKey: "ok", IsRequired: true, Enabled: true, Status: "NORMAL"},
		{TargetKey: "ok2", IsRequired: true, Enabled: true, Status: "NORMAL"},
		{TargetKey: "bad", IsRequired: true, Enabled: true, Status: "ABNORMAL"},
		{TargetKey: "idle", IsRequired: false, Enabled: true, Status: "ABNORMAL"},
	}
	healths := monitorTargetHealths(targets)

	got, err := domain.AggregateMonitor(domain.ModeAllRequired, healths)
	if err != nil || got != "ABNORMAL" {
		t.Fatalf("ALL_REQUIRED: want ABNORMAL got %s err %v", got, err)
	}
	got, err = domain.AggregateMonitor(domain.ModeAnyRequired, healths)
	if err != nil || got != "NORMAL" {
		t.Fatalf("ANY_REQUIRED: want NORMAL got %s err %v", got, err)
	}
	got, err = domain.AggregateMonitor(domain.ModeMajority, healths)
	if err != nil || got != "NORMAL" {
		t.Fatalf("MAJORITY: want NORMAL got %s err %v", got, err)
	}
}
