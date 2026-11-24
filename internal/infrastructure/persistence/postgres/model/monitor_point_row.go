package model

import (
	"time"

	portmonitor "github.com/ecakeman/alert/internal/application/port/monitor"
)

// MonitorPointColumns 是 system.service_monitor_points 的完整列清单（唯一来源）。
const MonitorPointColumns = `point_seq,service_id,target_id,target_key,target_type,observed_at,received_at,check_status,status_class,error_class,state_fingerprint,latency_ms,probe_generation`

// MonitorPointRow 是 system.service_monitor_points 行模型。
type MonitorPointRow struct {
	PointSeq         int64
	ServiceID        string
	TargetID         string
	TargetKey        string
	TargetType       string
	ObservedAt       time.Time
	ReceivedAt       time.Time
	CheckStatus      string
	StatusClass      string
	ErrorClass       string
	StateFingerprint string
	LatencyMS        *int
	ProbeGeneration  int64
}

// Scan 从一行读取 MonitorPointRow。
func (r MonitorPointRow) Scan(row Scannable) (MonitorPointRow, error) {
	if err := row.Scan(&r.PointSeq, &r.ServiceID, &r.TargetID, &r.TargetKey, &r.TargetType,
		&r.ObservedAt, &r.ReceivedAt, &r.CheckStatus, &r.StatusClass, &r.ErrorClass,
		&r.StateFingerprint, &r.LatencyMS, &r.ProbeGeneration); err != nil {
		return MonitorPointRow{}, err
	}
	return r, nil
}

// ToApp 转换为 application 值类型。
func (r MonitorPointRow) ToApp() portmonitor.MonitorPoint {
	return portmonitor.MonitorPoint{
		PointSeq: r.PointSeq, ServiceID: r.ServiceID, TargetID: r.TargetID,
		TargetKey: r.TargetKey, TargetType: r.TargetType,
		ObservedAt: r.ObservedAt, ReceivedAt: r.ReceivedAt,
		CheckStatus: r.CheckStatus, StatusClass: r.StatusClass, ErrorClass: r.ErrorClass,
		StateFingerprint: r.StateFingerprint, LatencyMS: r.LatencyMS, ProbeGeneration: r.ProbeGeneration,
	}
}

// MonitorPointRowFromApp 从 application 值类型构造行模型（不含自增 point_seq）。
func MonitorPointRowFromApp(p portmonitor.MonitorPoint) MonitorPointRow {
	return MonitorPointRow{
		ServiceID: p.ServiceID, TargetID: p.TargetID, TargetKey: p.TargetKey,
		TargetType: p.TargetType, ObservedAt: p.ObservedAt, ReceivedAt: p.ReceivedAt,
		CheckStatus: p.CheckStatus, StatusClass: p.StatusClass, ErrorClass: p.ErrorClass,
		StateFingerprint: p.StateFingerprint, LatencyMS: p.LatencyMS, ProbeGeneration: p.ProbeGeneration,
	}
}
