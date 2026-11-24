package dashboard

import (
	"context"
	"time"

	"github.com/ecakeman/alert/internal/application/port"
)

type Deps struct {
	UOW   port.UnitOfWork
	Repo  port.DashboardRepo
	Clock port.Clock
}

type Service struct{ deps Deps }

func New(deps Deps) *Service { return &Service{deps: deps} }

func (s *Service) Summary(ctx context.Context) (port.DashboardSummary, error) {
	var out port.DashboardSummary
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx port.Tx) error {
		var err error
		out, err = s.deps.Repo.Summary(ctx, tx, s.deps.Clock.Now().UTC(), 24*time.Hour)
		return err
	})
	return out, err
}
