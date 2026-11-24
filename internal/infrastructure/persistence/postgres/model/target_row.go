package model

import (
	"time"

	"github.com/ecakeman/alert/internal/application/port"
)

// TargetColumns 是 system.service_monitor_targets 的完整列清单（唯一来源）。
const TargetColumns = `target_id,service_id,target_key,target_name,address,target_type,config,interval_ms,timeout_ms,failure_threshold,recovery_threshold,is_required,enabled,status,consecutive_failures,consecutive_successes,last_check_at,config_generation,deleted_at,created_at,updated_at`

// TargetRow 是 system.service_monitor_targets 行模型。
type TargetRow struct {
	TargetID             string
	ServiceID            string
	TargetKey            string
	TargetName           string
	Address              string
	TargetType           string
	Config               []byte
	IntervalMS           int
	TimeoutMS            int
	FailureThreshold     int
	RecoveryThreshold    int
	IsRequired           bool
	Enabled              bool
	Status               string
	ConsecutiveFailures  int
	ConsecutiveSuccesses int
	LastCheckAt          *time.Time
	ConfigGeneration     int64
	DeletedAt            *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// Scan 从一行读取 TargetRow。
func (r TargetRow) Scan(row Scannable) (TargetRow, error) {
	if err := row.Scan(&r.TargetID, &r.ServiceID, &r.TargetKey, &r.TargetName, &r.Address, &r.TargetType,
		&r.Config, &r.IntervalMS, &r.TimeoutMS, &r.FailureThreshold, &r.RecoveryThreshold, &r.IsRequired,
		&r.Enabled, &r.Status, &r.ConsecutiveFailures, &r.ConsecutiveSuccesses, &r.LastCheckAt,
		&r.ConfigGeneration, &r.DeletedAt, &r.CreatedAt, &r.UpdatedAt); err != nil {
		return TargetRow{}, err
	}
	return r, nil
}

// ToApp 转换为 application 值类型。
func (r TargetRow) ToApp() port.MonitorTarget {
	return port.MonitorTarget{
		TargetID: r.TargetID, ServiceID: r.ServiceID, TargetKey: r.TargetKey,
		TargetName: r.TargetName, Address: r.Address, TargetType: r.TargetType,
		Config: r.Config, IntervalMS: r.IntervalMS, TimeoutMS: r.TimeoutMS,
		FailureThreshold: r.FailureThreshold, RecoveryThreshold: r.RecoveryThreshold,
		IsRequired: r.IsRequired, Enabled: r.Enabled, Status: r.Status,
		ConsecutiveFailures: r.ConsecutiveFailures, ConsecutiveSuccesses: r.ConsecutiveSuccesses,
		LastCheckAt: r.LastCheckAt, ConfigGeneration: r.ConfigGeneration,
		DeletedAt: r.DeletedAt, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

// TargetRowFromApp 从 application 值类型构造行模型。
func TargetRowFromApp(t port.MonitorTarget) TargetRow {
	return TargetRow{
		TargetID: t.TargetID, ServiceID: t.ServiceID, TargetKey: t.TargetKey,
		TargetName: t.TargetName, Address: t.Address, TargetType: t.TargetType,
		Config: t.Config, IntervalMS: t.IntervalMS, TimeoutMS: t.TimeoutMS,
		FailureThreshold: t.FailureThreshold, RecoveryThreshold: t.RecoveryThreshold,
		IsRequired: t.IsRequired, Enabled: t.Enabled, Status: t.Status,
		ConsecutiveFailures: t.ConsecutiveFailures, ConsecutiveSuccesses: t.ConsecutiveSuccesses,
		LastCheckAt: t.LastCheckAt, ConfigGeneration: t.ConfigGeneration,
		DeletedAt: t.DeletedAt, CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt,
	}
}
