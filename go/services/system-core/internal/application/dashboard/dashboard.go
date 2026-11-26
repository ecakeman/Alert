package dashboard

import (
	"context"
	"time"

	portdashboard "github.com/ecakeman/alert/services/system-core/internal/application/port/dashboard"
	portshared "github.com/ecakeman/alert/services/system-core/internal/application/port/shared"
)

type Deps struct {
	UOW     portshared.UnitOfWork
	Repo    portdashboard.DashboardRepo
	Episode portdashboard.EpisodeSource
	Clock   portshared.Clock
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
	if err != nil || s.deps.Episode == nil {
		return out, err
	}
	counts, err := s.deps.Episode.Counts(ctx)
	if err != nil {
		return out, err
	}
	out.Episodes = counts
	recent, err := s.deps.Episode.Recent(ctx, 5)
	if err != nil {
		return out, err
	}
	if enricher, ok := s.deps.Repo.(portdashboard.RecentEnricher); ok {
		if err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
			var eerr error
			recent, eerr = enricher.EnrichRecent(ctx, tx, recent)
			return eerr
		}); err != nil {
			return out, err
		}
	}
	out.Recent = recent
	return out, nil
}
