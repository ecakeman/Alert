package monitor

import (
	"context"
	portshared "github.com/ecakeman/alert/internal/application/port/shared"
	"time"
)

type MonitorPoint struct {
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

// TargetSnapshot 是探测调度所需的目标快照（不含存储时间字段）。
type TargetSnapshot struct {
	TargetID             string
	ServiceID            string
	TargetKey            string
	TargetType           string
	Address              string
	Config               []byte
	IntervalMS           int
	TimeoutMS            int
	FailureThreshold     int
	RecoveryThreshold    int
	IsRequired           bool
	Status               string
	ConsecutiveFailures  int
	ConsecutiveSuccesses int
	LastCheckAt          *time.Time
	ConfigGeneration     int64
}

type MonitorPointRepo interface {
	Append(ctx context.Context, tx portshared.Tx, p MonitorPoint) (int64, error)
	ListByBound(ctx context.Context, tx portshared.Tx, serviceID string, fromSeq, toSeq int64, limit int) ([]MonitorPoint, error)
	LatestSeqByService(ctx context.Context, tx portshared.Tx, serviceID string) (int64, error)
}

// TargetScheduleSource 是探测调度读取“启用且到期目标快照”的窄端口；
// 由 monitor 用例实现，bootstrap 注入 probe（T-S1.13 接线）。
type TargetScheduleSource interface {
	DueTargets(ctx context.Context, now time.Time) ([]TargetSnapshot, error)
}
