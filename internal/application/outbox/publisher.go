package outbox

import (
	"context"
	"time"

	"github.com/ecakeman/alert/internal/application/port"
)

type Deps struct {
	UOW          port.UnitOfWork
	Outbox       port.OutboxRepo
	Sender       port.EventSender
	Clock        port.Clock
	Owner        string
	Batch        int
	ClaimLease   time.Duration
	PollInterval time.Duration
}

type Publisher struct{ deps Deps }

func New(deps Deps) *Publisher { return &Publisher{deps: deps} }

// RunOnce 执行一次三段式：claim 短事务 → 事务外发送 → ack/失败短事务。
func (p *Publisher) RunOnce(ctx context.Context) (int, error) {
	if p.deps.ClaimLease <= 0 {
		p.deps.ClaimLease = 30 * time.Second
	}
	if p.deps.Batch <= 0 {
		p.deps.Batch = 16
	}

	var batch []port.OutboxRecord
	err := p.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx port.Tx) error {
		var err error
		batch, err = p.deps.Outbox.Claim(ctx, tx, p.deps.Owner, p.deps.Clock.Now().Add(p.deps.ClaimLease), p.deps.Batch)
		return err
	})
	if err != nil {
		return 0, err
	}
	if len(batch) == 0 {
		return 0, nil
	}

	sent := 0
	for _, rec := range batch {
		e := rec.Event
		sendCtx, cancel := context.WithTimeout(ctx, p.deps.ClaimLease/2)
		sendErr := p.deps.Sender.Send(sendCtx, e)
		cancel()

		if sendErr != nil {
			nextRetry := p.deps.Clock.Now().Add(p.deps.PollInterval)
			if nextRetry.Before(p.deps.Clock.Now()) {
				nextRetry = p.deps.Clock.Now().Add(time.Second)
			}
			_ = p.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx port.Tx) error {
				return p.deps.Outbox.MarkFailed(ctx, tx, e.EventID, nextRetry, sendErr.Error())
			})
			continue
		}

		if ackErr := p.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx port.Tx) error {
			return p.deps.Outbox.MarkPublished(ctx, tx, e.EventID, p.deps.Clock.Now())
		}); ackErr == nil {
			sent++
		}
	}
	return sent, nil
}

// Run 轮询发布，直到 ctx 取消。
func (p *Publisher) Run(ctx context.Context) error {
	interval := p.deps.PollInterval
	if interval <= 0 {
		interval = 500 * time.Millisecond
	}
	ticker := time.NewTicker(interval)
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
