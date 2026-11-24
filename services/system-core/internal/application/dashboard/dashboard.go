package dashboard

import (
	"context"
	"time"

	portdashboard "github.com/ecakeman/alert/services/system-core/internal/application/port/dashboard"
	portshared "github.com/ecakeman/alert/services/system-core/internal/application/port/shared"
)

type Deps struct {
	UOW   portshared.UnitOfWork
	Repo  portdashboard.DashboardRepo
	Clock portshared.Clock
}

type Service struct{ deps Deps }

func New(deps Deps) *Service { return &Service{deps: deps} }

func (s *Service) Summary(ctx context.Context) (portdashboard.DashboardSummary, error) {
	var out portdashboard.DashboardSummary
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		var err error
		out, err = s.deps.Repo.Summary(ctx, tx, s.deps.Clock.Now().UTC(), 24*time.Hour)
		return err
	})
	return out, err
}
