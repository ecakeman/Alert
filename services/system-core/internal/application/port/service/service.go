package service

import (
	"context"
	portshared "github.com/ecakeman/alert/services/system-core/internal/application/port/shared"
	"time"
)

type Service struct {
	ServiceID              string
	ServiceKey             string
	Name                   string
	Description            string
	Enabled                bool
	MonitorAggregationMode string
	AuthorityProducerID    string
}

type MonitorTarget struct {
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
}

type SourceState struct {
	ServiceID  string
	SourceType string
	Status     string
	Reason     string
}

type ServiceFilter struct {
	Query  string
	Status string
	Limit  int
	Offset int
}

type ServicePatch struct {
	Name                   *string
	Description            *string
	Enabled                *bool
	MonitorAggregationMode *string
}

type TargetPatch struct {
	TargetName        *string
	Address           *string
	Config            []byte
	IntervalMS        *int
	TimeoutMS         *int
	FailureThreshold  *int
	RecoveryThreshold *int
	IsRequired        *bool
	Enabled           *bool
}

type ServiceRepo interface {
	Create(ctx context.Context, tx portshared.Tx, s Service) error
	Get(ctx context.Context, tx portshared.Tx, serviceID string) (Service, error)
	GetByKey(ctx context.Context, tx portshared.Tx, serviceKey string) (Service, error)
	List(ctx context.Context, tx portshared.Tx, f ServiceFilter) ([]Service, error)
	Count(ctx context.Context, tx portshared.Tx, f ServiceFilter) (int, error)

	Patch(ctx context.Context, tx portshared.Tx, serviceID string, p ServicePatch) error
	SoftDelete(ctx context.Context, tx portshared.Tx, serviceID string, at time.Time) error
}

type TargetRepo interface {
	Create(ctx context.Context, tx portshared.Tx, t MonitorTarget) error
	Get(ctx context.Context, tx portshared.Tx, targetID string) (MonitorTarget, error)
	ListByService(ctx context.Context, tx portshared.Tx, serviceID string, includeDeleted bool) ([]MonitorTarget, error)
	ListEnabled(ctx context.Context, tx portshared.Tx) ([]MonitorTarget, error)
	Patch(ctx context.Context, tx portshared.Tx, targetID string, p TargetPatch) error
	UpdateProbeResult(ctx context.Context, tx portshared.Tx, targetID, status string, failures, successes int, lastCheckAt time.Time) error
	SoftDelete(ctx context.Context, tx portshared.Tx, targetID string, at time.Time) error
}

type SourceStateRepo interface {
	Get(ctx context.Context, tx portshared.Tx, serviceID, sourceType string) (SourceState, error)
	ListByService(ctx context.Context, tx portshared.Tx, serviceID string) ([]SourceState, error)
	Upsert(ctx context.Context, tx portshared.Tx, s SourceState) error
}
