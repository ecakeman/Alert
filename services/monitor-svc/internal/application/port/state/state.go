package state

import (
	"context"
	"time"

	portshared "github.com/ecakeman/alert/services/monitor-svc/internal/application/port/shared"
)

// TargetState 是 monitor 自产的单个目标探测状态。
type TargetState struct {
	TargetID             string
	ServiceID            string
	Status               string
	ConsecutiveFailures  int
	ConsecutiveSuccesses int
	LastCheckAt          *time.Time
	UpdatedAt            time.Time
}

// SourceState 是 monitor 自产的服务级 MONITOR 来源状态。
type SourceState struct {
	ServiceID       string
	Status          string
	AggregationMode string
	UpdatedAt       time.Time
}

// Repo 是探测状态表的持久化端口。
type Repo interface {
	GetTarget(ctx context.Context, tx portshared.Tx, targetID string) (TargetState, error)
	ListTargets(ctx context.Context, tx portshared.Tx, serviceID string) ([]TargetState, error)
	UpsertTarget(ctx context.Context, tx portshared.Tx, s TargetState, at time.Time) error

	GetSource(ctx context.Context, tx portshared.Tx, serviceID string) (SourceState, error)
	UpsertSource(ctx context.Context, tx portshared.Tx, s SourceState, at time.Time) error
}