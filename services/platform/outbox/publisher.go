package outbox

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Sender 是 Kafka 发送端口；PublishTopic 由 Publisher 注入，事务外调用。
type Sender interface {
	Send(ctx context.Context, topic string, rec Record) error
}

// Store 是 Publisher 所需的最小仓储接口（Repo 实现之）。
type Store interface {
	Claim(ctx context.Context, db DBTX, owner string, until time.Time, limit int) ([]Record, error)
	Ack(ctx context.Context, db DBTX, eventID string, at time.Time) error
	Fail(ctx context.Context, db DBTX, eventID string, nextRetryAt time.Time, errMsg string) error
}

// TxBeginner 抽象连接池事务入口（pgxpool.Pool 实现之）。
type TxBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Deps 是发布器的全部依赖；Owner / PublishTopic 由调用方注入。
type Deps struct {
	Store        Store
	Tx           TxBeginner
	Sender       Sender
	Owner        string
	PublishTopic string
	ClaimLease   time.Duration
	Batch        int
	PollInterval time.Duration
	Now          func() time.Time
}

// Publisher 执行 claim → 事务外发送 → ack/失败三段式。
type Publisher struct {
	deps Deps
}

// New 校验并补齐默认值；ClaimLease 必须足以容纳单条发送超时（ClaimLease/2）。
func New(deps Deps) (*Publisher, error) {
	if deps.Store == nil {
		return nil, fmt.Errorf("outbox publisher: Store is required")
	}
	if deps.Tx == nil {
		return nil, fmt.Errorf("outbox publisher: Tx is required")
	}
	if deps.Sender == nil {
		return nil, fmt.Errorf("outbox publisher: Sender is required")
	}
	if deps.Owner == "" {
		return nil, fmt.Errorf("outbox publisher: Owner is required")
	}
	if deps.PublishTopic == "" {
		return nil, fmt.Errorf("outbox publisher: PublishTopic is required")
	}
	if deps.ClaimLease <= 0 {
		deps.ClaimLease = 30 * time.Second
	}
	if deps.Batch <= 0 {
		deps.Batch = 16
	}
	if deps.PollInterval <= 0 {
		deps.PollInterval = 500 * time.Millisecond
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	return &Publisher{deps: deps}, nil
}

// SendTimeout 返回单条发送超时（ClaimLease/2）。
func (p *Publisher) SendTimeout() time.Duration { return p.deps.ClaimLease / 2 }

// RunOnce 执行一次三段式：claim 短事务 → 事务外发送 → ack/失败短事务。
func (p *Publisher) RunOnce(ctx context.Context) (int, error) {
	var batch []Record
	if err := p.inTx(ctx, func(tx pgx.Tx) error {
		var err error
		batch, err = p.deps.Store.Claim(ctx, tx, p.deps.Owner, p.deps.Now().Add(p.deps.ClaimLease), p.deps.Batch)
		return err
	}); err != nil {
		return 0, err
	}
	if len(batch) == 0 {
		return 0, nil
	}

	sent := 0
	for _, rec := range batch {
		sendCtx, cancel := context.WithTimeout(ctx, p.SendTimeout())
		sendErr := p.deps.Sender.Send(sendCtx, p.deps.PublishTopic, rec)
		cancel()

		now := p.deps.Now()
		if sendErr != nil {
			nextRetry := now.Add(p.deps.PollInterval)
			if !nextRetry.After(now) {
				nextRetry = now.Add(time.Second)
			}
			_ = p.inTx(ctx, func(tx pgx.Tx) error {
				return p.deps.Store.Fail(ctx, tx, rec.EventID, nextRetry, sendErr.Error())
			})
			continue
		}
		if ackErr := p.inTx(ctx, func(tx pgx.Tx) error {
			return p.deps.Store.Ack(ctx, tx, rec.EventID, now)
		}); ackErr == nil {
			sent++
		}
	}
	return sent, nil
}

// Run 轮询发布，直到 ctx 取消。
func (p *Publisher) Run(ctx context.Context) error {
	ticker := time.NewTicker(p.deps.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			_, _ = p.RunOnce(ctx)
		}
	}
}

func (p *Publisher) inTx(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := p.deps.Tx.Begin(ctx)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	return tx.Commit(ctx)
}
