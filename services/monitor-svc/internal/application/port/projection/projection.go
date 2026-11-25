package projection

import (
	"context"
	"time"

	portshared "github.com/ecakeman/alert/services/monitor-svc/internal/application/port/shared"
)

// Target 是调度所需的 registry 目标配置在 monitor 侧的只读投影。
// 不含 status/consecutive_*（探测结果回传 registry，不在投影保存）。
type Target struct {
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

// Repo 是投影表的持久化端口。
type Repo interface {
	// Upsert 写入/更新投影；仅当 incoming 的 config_generation 不小于现值时覆盖（乱序保护）。
	Upsert(ctx context.Context, tx portshared.Tx, t Target, syncedAt time.Time) error
	// MarkDeleted 把该 service 下不在快照内的未删除目标标记删除（target 删除事件语义）。
	MarkDeleted(ctx context.Context, tx portshared.Tx, serviceID string, keepTargetIDs []string, at time.Time) error
	// MarkServiceDeleted 把该 service 全部未删除目标标记删除（service 停用/软删）。
	MarkServiceDeleted(ctx context.Context, tx portshared.Tx, serviceID string, at time.Time) error

	Get(ctx context.Context, tx portshared.Tx, targetID string) (Target, error)
	ListByService(ctx context.Context, tx portshared.Tx, serviceID string, includeDeleted bool) ([]Target, error)
	ListDue(ctx context.Context, tx portshared.Tx, now time.Time) ([]Target, error)
	UpdateLastProbeAt(ctx context.Context, tx portshared.Tx, targetID string, at time.Time) error
}