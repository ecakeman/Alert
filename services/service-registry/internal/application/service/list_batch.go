package service

import (
	"context"

	portService "github.com/ecakeman/alert/services/service-registry/internal/application/port/service"
	portshared "github.com/ecakeman/alert/services/service-registry/internal/application/port/shared"
)

// ListByIDs 批量返回服务（BFF/域内批量补齐用，单次 ≤200 由调用方校验）。
func (s *Service) ListByIDs(ctx context.Context, ids []string) ([]portService.Service, error) {
	var out []portService.Service
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		var err error
		out, err = s.deps.Services.ListByIDs(ctx, tx, ids)
		return err
	})
	return out, err
}

// Count 返回符合过滤条件的服务总数。
func (s *Service) Count(ctx context.Context, f portService.ServiceFilter) (int, error) {
	var total int
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		var err error
		total, err = s.deps.Services.Count(ctx, tx, f)
		return err
	})
	return total, err
}

// GetSourceStates 返回服务两条来源状态（Get 的补充端点用）。
func (s *Service) GetSourceStates(ctx context.Context, serviceID string) ([]portService.SourceState, error) {
	return s.ListSourceStates(ctx, serviceID)
}
