package postgres

import (
	"context"
	"time"

	"github.com/ecakeman/alert/internal/application/port"
)

type idempotencyRepo struct{}

func NewIdempotencyRepo() port.ConsumerIdempotencyRepo { return idempotencyRepo{} }

// Reserve：以 (consumer, idempotency_key) 唯一键占位；已存在返回 false。
func (idempotencyRepo) Reserve(ctx context.Context, tx port.Tx, idem port.ConsumerIdempotency) (bool, error) {
	tag, err := txFrom(tx).Exec(ctx, `
INSERT INTO system.consumer_idempotency(consumer,idempotency_key,event_id,task_id,generation,processed_at)
VALUES($1,$2,$3,$4,$5,now())
ON CONFLICT (consumer, idempotency_key) DO NOTHING`,
		idem.Consumer, idem.IdempotencyKey, idem.EventID, idem.TaskID, idem.Generation)
	if err != nil {
		return false, Translate(err)
	}
	return tag.RowsAffected() == 1, nil
}

func (idempotencyRepo) MarkDone(ctx context.Context, tx port.Tx, consumer, key, resultCode string, at time.Time) error {
	_, err := txFrom(tx).Exec(ctx, `
UPDATE system.consumer_idempotency SET result_code=$3, processed_at=$4
WHERE consumer=$1 AND idempotency_key=$2`, consumer, key, resultCode, utc(at))
	return Translate(err)
}
