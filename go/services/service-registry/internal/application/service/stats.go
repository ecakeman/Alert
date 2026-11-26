package service

import (
	"context"

	portService "github.com/ecakeman/alert/services/service-registry/internal/application/port/service"
	portshared "github.com/ecakeman/alert/services/service-registry/internal/application/port/shared"
)

// RegistryStats 是本域统计：服务总数与目标状态计数。
type RegistryStats struct {
	Services int            `json:"services"`
	Targets  map[string]int `json:"targets"`
}

// Stats 返回本域计数（不含跨域字段）。
func (s *Service) Stats(ctx context.Context) (RegistryStats, error) {
	out := RegistryStats{Targets: map[string]int{"NORMAL": 0, "ABNORMAL": 0, "UNKNOWN": 0}}
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		count, err := s.deps.Services.Count(ctx, tx, portService.ServiceFilter{})
		if err != nil {
			return err
		}
		out.Services = count
		counts, err := s.deps.Targets.StatusCounts(ctx, tx)
		if err != nil {
			return err
		}
		out.Targets = counts
		return nil
	})
	return out, err
}

// ListSourceStates 返回服务两条来源状态（立案解除判定取数）。
func (s *Service) ListSourceStates(ctx context.Context, serviceID string) ([]portService.SourceState, error) {
	var out []portService.SourceState
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		var err error
		out, err = s.deps.Sources.ListByService(ctx, tx, serviceID)
		return err
	})
	return out, err
}
