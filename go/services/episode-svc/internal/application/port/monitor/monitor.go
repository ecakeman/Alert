package monitor

import (
	"context"
	"time"
)

// MonitorPoint 是 monitor-svc 点位的最小只读投影（P4 只参与指纹计算）。
type MonitorPoint struct {
	PointSeq         int64
	ServiceID        string
	TargetID         string
	TargetKey        string
	TargetType       string
	ObservedAt       time.Time
	CheckStatus      string
	StatusClass      string
	ErrorClass       string
	StateFingerprint string
	LatencyMS        *int
	ProbeGeneration  int64
}

// RemoteReader 是 episode 域对 monitor-svc 的最小读取端口（事务外调用）。
type RemoteReader interface {
	LatestSeqByService(ctx context.Context, serviceID string) (int64, error)
	ListByBound(ctx context.Context, serviceID string, fromSeq, toSeq int64, limit int) ([]MonitorPoint, error)
}