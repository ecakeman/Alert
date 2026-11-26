package service

import (
	"context"
	"time"

	"github.com/ecakeman/alert/services/system-core/internal/application/apperr"
	portService "github.com/ecakeman/alert/services/system-core/internal/application/port/service"
	portshared "github.com/ecakeman/alert/services/system-core/internal/application/port/shared"
)

// ServiceSummaryData 是列表/详情所需的聚合视图（含双来源状态与目标统计）。
type ServiceSummaryData struct {
	Service         portService.Service
	MonitorStatus   string
	AppStatus       string
	TargetCount     int
	LastCheckAt     *time.Time
	ActiveEpisodeID *string
}

type ServiceViewData struct {
	Summary ServiceSummaryData
	Targets []portService.MonitorTarget
}

func (s *Service) enrich(ctx context.Context, tx portshared.Tx, svc portService.Service) (ServiceSummaryData, error) {
	sources, err := s.deps.Sources.ListByService(ctx, tx, svc.ServiceID)
	if err != nil {
		return ServiceSummaryData{}, err
	}
	targets, err := s.deps.Targets.ListByService(ctx, tx, svc.ServiceID, false)
	if err != nil {
		return ServiceSummaryData{}, err
	}

	monitor, app := "UNKNOWN", "UNKNOWN"
	for _, st := range sources {
		switch st.SourceType {
		case "MONITOR":
			monitor = st.Status
		case "APP":
			app = st.Status
		}
	}

	var lastCheck *time.Time
	for _, t := range targets {
		if t.LastCheckAt != nil && (lastCheck == nil || t.LastCheckAt.After(*lastCheck)) {
			lastCheck = t.LastCheckAt
		}
	}

	var activeEpisodeID *string
	ep, err := s.deps.Episodes.GetActiveByService(ctx, tx, svc.ServiceID)
	if err == nil {
		activeEpisodeID = &ep.EpisodeID
	} else if !isAppCode(err, apperr.CodeNotFound) {
		return ServiceSummaryData{}, err
	}

	return ServiceSummaryData{
		Service: svc, MonitorStatus: monitor, AppStatus: app,
		TargetCount: len(targets), LastCheckAt: lastCheck, ActiveEpisodeID: activeEpisodeID,
	}, nil
}

// ListSummaries 返回列表聚合：同一事务内先 Count 再 List，total 与过滤口径一致。
func (s *Service) ListSummaries(ctx context.Context, f portService.ServiceFilter) (ServiceSummaryPage, error) {
	var page ServiceSummaryPage
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		total, err := s.deps.Services.Count(ctx, tx, f)
		if err != nil {
			return err
		}
		svcs, err := s.deps.Services.List(ctx, tx, f)
		if err != nil {
			return err
		}
		page.Page = portshared.Page{Total: total, Limit: f.Limit, Offset: f.Offset}
		for _, svc := range svcs {
			d, err := s.enrich(ctx, tx, svc)
			if err != nil {
				return err
			}
			page.Items = append(page.Items, d)
		}
		return nil
	})
	return page, err
}

// GetView 返回详情聚合（服务 + 目标 + 来源状态）。
func (s *Service) GetView(ctx context.Context, serviceID string) (ServiceViewData, error) {
	var view ServiceViewData
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		svc, err := s.deps.Services.Get(ctx, tx, serviceID)
		if err != nil {
			return err
		}
		targets, err := s.deps.Targets.ListByService(ctx, tx, serviceID, false)
		if err != nil {
			return err
		}
		d, err := s.enrich(ctx, tx, svc)
		if err != nil {
			return err
		}
		view = ServiceViewData{Summary: d, Targets: targets}
		return nil
	})
	return view, err
}
