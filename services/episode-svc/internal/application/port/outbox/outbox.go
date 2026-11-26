package outbox

import (
	"context"
	"time"

	"github.com/ecakeman/alert/services/contracts"
	portshared "github.com/ecakeman/alert/services/episode-svc/internal/application/port/shared"
)

// OutboxRecord 是 Outbox 存储行：事件信封 + 投递控制字段；不进入线上契约。
type OutboxRecord struct {
	Event           contracts.Event
	DedupeKey       *string
	PublishedAt     *time.Time
	PublishAttempts int
	NextRetryAt     *time.Time
	ClaimOwner      *string
	ClaimUntil      *time.Time
	LastError       string
}

type ConsumerIdempotency struct {
	Consumer       string
	IdempotencyKey string
	EventID        *string
	TaskID         *string
	Generation     *int64
	ResultCode     string
}

type OutboxRepo interface {
	Append(ctx context.Context, tx portshared.Tx, event contracts.Event, dedupeKey *string) error
	Claim(ctx context.Context, tx portshared.Tx, owner string, until time.Time, limit int) ([]OutboxRecord, error)
	MarkPublished(ctx context.Context, tx portshared.Tx, eventID string, at time.Time) error
	MarkFailed(ctx context.Context, tx portshared.Tx, eventID string, nextRetryAt time.Time, errMsg string) error
}

type ConsumerIdempotencyRepo interface {
	Reserve(ctx context.Context, tx portshared.Tx, idem ConsumerIdempotency) (bool, error)
	MarkDone(ctx context.Context, tx portshared.Tx, consumer, key, resultCode string, at time.Time) error
}