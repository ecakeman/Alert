package service

import (
	"context"
	"time"

	"github.com/ecakeman/alert/internal/application/apperr"
	"github.com/ecakeman/alert/internal/application/port"
)

// ServiceSummaryData 是列表/详情所需的聚合视图（含双来源状态与目标统计）。
type ServiceSummaryData struct {
	Service         port.Service
	MonitorStatus   string
	AppStatus       string
	TargetCount     int
	LastCheckAt     *time.Time
	ActiveEpisodeID *string
}

type ServiceViewData struct {
	Summary ServiceSummaryData
	Targets []port.MonitorTarget
}

func (s *Service) enrich(ctx context.Context, tx port.Tx, svc port.Service) (ServiceSummaryData, error) {
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

// ListSummaries 返回列表聚合（含来源状态/目标数/最近检查/活动故障）。
func (s *Service) ListSummaries(ctx context.Context, f port.ServiceFilter) ([]ServiceSummaryData, error) {
	var out []ServiceSummaryData
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx port.Tx) error {
		svcs, err := s.deps.Services.List(ctx, tx, f)
		if err != nil {
			return err
		}
		for _, svc := range svcs {
			d, err := s.enrich(ctx, tx, svc)
			if err != nil {
				return err
			}
			out = append(out, d)
		}
		return nil
	})
	return out, err
}

// GetView 返回详情聚合（服务 + 目标 + 来源状态）。
func (s *Service) GetView(ctx context.Context, serviceID string) (ServiceViewData, error) {
	var view ServiceViewData
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx port.Tx) error {
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
