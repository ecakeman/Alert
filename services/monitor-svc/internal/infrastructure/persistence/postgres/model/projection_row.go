package model

import (
	"time"

	"github.com/ecakeman/alert/services/monitor-svc/internal/application/port/projection"
)

// ProjectionColumns 是 monitor.monitor_target_projection 的完整列清单（唯一来源）。
const ProjectionColumns = `target_id,service_id,target_key,target_type,address,interval_ms,timeout_ms,failure_threshold,recovery_threshold,is_required,enabled,config_generation,monitor_aggregation_mode,last_probe_at,deleted_at,synced_at`

// ProjectionRow 是 monitor.monitor_target_projection 行模型。
type ProjectionRow struct {
	TargetID               string
	ServiceID              string
	TargetKey              string
	TargetType             string
	Address                string
	IntervalMS             int
	TimeoutMS              int
	FailureThreshold       int
	RecoveryThreshold      int
	IsRequired             bool
	Enabled                bool
	ConfigGeneration       int64
	MonitorAggregationMode string
	LastProbeAt            *time.Time
	DeletedAt              *time.Time
	SyncedAt               time.Time
}

// Scan 从一行读取 ProjectionRow。
func (r ProjectionRow) Scan(row Scannable) (ProjectionRow, error) {
	if err := row.Scan(&r.TargetID, &r.ServiceID, &r.TargetKey, &r.TargetType, &r.Address,
		&r.IntervalMS, &r.TimeoutMS, &r.FailureThreshold, &r.RecoveryThreshold,
		&r.IsRequired, &r.Enabled, &r.ConfigGeneration, &r.MonitorAggregationMode,
		&r.LastProbeAt, &r.DeletedAt, &r.SyncedAt); err != nil {
		return ProjectionRow{}, err
	}
	return r, nil
}

// ToApp 转换为 application 值类型。
func (r ProjectionRow) ToApp() projection.Target {
	return projection.Target{
		TargetID: r.TargetID, ServiceID: r.ServiceID, TargetKey: r.TargetKey,
		TargetType: r.TargetType, Address: r.Address, IntervalMS: r.IntervalMS, TimeoutMS: r.TimeoutMS,
		FailureThreshold: r.FailureThreshold, RecoveryThreshold: r.RecoveryThreshold,
		IsRequired: r.IsRequired, Enabled: r.Enabled, ConfigGeneration: r.ConfigGeneration,
		MonitorAggregationMode: r.MonitorAggregationMode, LastProbeAt: r.LastProbeAt,
		DeletedAt: r.DeletedAt, SyncedAt: r.SyncedAt,
	}
}