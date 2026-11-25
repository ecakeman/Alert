package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/ecakeman/alert/services/service-registry/internal/application/apperr"
	portoutbox "github.com/ecakeman/alert/services/service-registry/internal/application/port/outbox"
	portService "github.com/ecakeman/alert/services/service-registry/internal/application/port/service"
	portshared "github.com/ecakeman/alert/services/service-registry/internal/application/port/shared"
	"github.com/ecakeman/alert/services/service-registry/internal/contracts"
	"github.com/ecakeman/alert/services/service-registry/internal/domain"
)

// Deps 是用例依赖的端口集合。
type Deps struct {
	UOW      portshared.UnitOfWork
	Services portService.ServiceRepo
	Targets  portService.TargetRepo
	Sources  portService.SourceStateRepo
	Outbox   portoutbox.OutboxRepo
	Clock    portshared.Clock
	IDs      portshared.IDGen
}

type Service struct{ deps Deps }

func New(deps Deps) *Service { return &Service{deps: deps} }

func isAppCode(err error, code string) bool {
	var ae *apperr.Error
	return errors.As(err, &ae) && ae.Code == code
}

// Create：服务 + 两条来源状态 + 初始目标 + Outbox，同一事务。
func (s *Service) Create(ctx context.Context, in CreateServiceInput) (ServiceView, error) {
	if in.ServiceKey == "" || in.Name == "" || in.AuthorityProducerID == "" {
		return ServiceView{}, apperr.Validation("service_key, name, authority_producer_id are required")
	}
	mode := in.MonitorAggregationMode
	if mode == "" {
		mode = "ALL_REQUIRED"
	}
	if mode != domain.ModeAllRequired && mode != domain.ModeAnyRequired && mode != domain.ModeMajority {
		return ServiceView{}, apperr.Validation("invalid monitor_aggregation_mode")
	}

	var view ServiceView
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		if _, err := s.deps.Services.GetByKey(ctx, tx, in.ServiceKey); err == nil {
			return apperr.AlreadyExists("service_key already exists")
		} else if !isAppCode(err, apperr.CodeNotFound) {
			return err
		}

		svcID := s.deps.IDs.NewID()
		now := s.deps.Clock.Now().UTC()
		svc := portService.Service{
			ServiceID: svcID, ServiceKey: in.ServiceKey, Name: in.Name, Description: in.Description,
			Enabled: true, MonitorAggregationMode: mode, AuthorityProducerID: in.AuthorityProducerID,
		}
		if err := s.deps.Services.Create(ctx, tx, svc); err != nil {
			return err
		}
		for _, st := range []string{"MONITOR", "APP"} {
			if err := s.deps.Sources.Upsert(ctx, tx, portService.SourceState{
				ServiceID: svcID, SourceType: st, Status: "UNKNOWN",
			}); err != nil {
				return err
			}
		}

		var targets []portService.MonitorTarget
		for _, ti := range in.Targets {
			t := s.newTarget(svcID, ti, now)
			if err := s.deps.Targets.Create(ctx, tx, t); err != nil {
				return err
			}
			targets = append(targets, t)
		}

		if err := s.recomputeMonitor(ctx, tx, svcID, mode); err != nil {
			return err
		}
		if err := s.appendServiceStateChanged(ctx, tx, svcID); err != nil {
			return err
		}
		view = ServiceView{Service: svc, Targets: targets}
		return nil
	})
	return view, err
}

func (s *Service) List(ctx context.Context, f portService.ServiceFilter) ([]portService.Service, error) {
	var out []portService.Service
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		var err error
		out, err = s.deps.Services.List(ctx, tx, f)
		return err
	})
	return out, err
}

func (s *Service) Get(ctx context.Context, serviceID string) (ServiceView, error) {
	var view ServiceView
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		svc, err := s.deps.Services.Get(ctx, tx, serviceID)
		if err != nil {
			return err
		}
		targets, err := s.deps.Targets.ListByService(ctx, tx, serviceID, false)
		if err != nil {
			return err
		}
		view = ServiceView{Service: svc, Targets: targets}
		return nil
	})
	return view, err
}

func (s *Service) Patch(ctx context.Context, serviceID string, p portService.ServicePatch) (ServiceView, error) {
	if p.Name == nil && p.Description == nil && p.Enabled == nil && p.MonitorAggregationMode == nil {
		return ServiceView{}, apperr.Validation("no updatable fields")
	}
	if p.MonitorAggregationMode != nil {
		m := *p.MonitorAggregationMode
		if m != domain.ModeAllRequired && m != domain.ModeAnyRequired && m != domain.ModeMajority {
			return ServiceView{}, apperr.Validation("invalid monitor_aggregation_mode")
		}
	}
	var view ServiceView
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		if err := s.deps.Services.Patch(ctx, tx, serviceID, p); err != nil {
			return err
		}
		svc, err := s.deps.Services.Get(ctx, tx, serviceID)
		if err != nil {
			return err
		}
		targets, err := s.deps.Targets.ListByService(ctx, tx, serviceID, false)
		if err != nil {
			return err
		}
		if err := s.appendServiceStateChanged(ctx, tx, serviceID); err != nil {
			return err
		}
		view = ServiceView{Service: svc, Targets: targets}
		return nil
	})
	return view, err
}

func (s *Service) SoftDelete(ctx context.Context, serviceID string) error {
	return s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		if _, err := s.deps.Services.Get(ctx, tx, serviceID); err != nil {
			return err
		}
		if err := s.deps.Services.SoftDelete(ctx, tx, serviceID, s.deps.Clock.Now().UTC()); err != nil {
			return err
		}
		return s.appendServiceStateChanged(ctx, tx, serviceID)
	})
}

func (s *Service) ListTargets(ctx context.Context, serviceID string) ([]portService.MonitorTarget, error) {
	var out []portService.MonitorTarget
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		if _, err := s.deps.Services.Get(ctx, tx, serviceID); err != nil {
			return err
		}
		var err error
		out, err = s.deps.Targets.ListByService(ctx, tx, serviceID, false)
		return err
	})
	return out, err
}

func (s *Service) CreateTarget(ctx context.Context, serviceID string, in TargetInput) (portService.MonitorTarget, error) {
	var t portService.MonitorTarget
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		svc, err := s.deps.Services.Get(ctx, tx, serviceID)
		if err != nil {
			return err
		}
		now := s.deps.Clock.Now().UTC()
		t = s.newTarget(serviceID, in, now)
		if err := s.deps.Targets.Create(ctx, tx, t); err != nil {
			return err
		}
		if err := s.recomputeMonitor(ctx, tx, serviceID, svc.MonitorAggregationMode); err != nil {
			return err
		}
		return s.appendServiceStateChanged(ctx, tx, serviceID)
	})
	return t, err
}

func (s *Service) PatchTarget(ctx context.Context, targetID string, p portService.TargetPatch) (portService.MonitorTarget, error) {
	var t portService.MonitorTarget
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		if err := s.deps.Targets.Patch(ctx, tx, targetID, p); err != nil {
			return err
		}
		var err error
		t, err = s.deps.Targets.Get(ctx, tx, targetID)
		if err != nil {
			return err
		}
		svc, err := s.deps.Services.Get(ctx, tx, t.ServiceID)
		if err != nil {
			return err
		}
		if err := s.recomputeMonitor(ctx, tx, t.ServiceID, svc.MonitorAggregationMode); err != nil {
			return err
		}
		return s.appendServiceStateChanged(ctx, tx, t.ServiceID)
	})
	return t, err
}

func (s *Service) DeleteTarget(ctx context.Context, targetID string) error {
	return s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		t, err := s.deps.Targets.Get(ctx, tx, targetID)
		if err != nil {
			return err
		}
		if err := s.deps.Targets.SoftDelete(ctx, tx, targetID, s.deps.Clock.Now().UTC()); err != nil {
			return err
		}
		svc, err := s.deps.Services.Get(ctx, tx, t.ServiceID)
		if err != nil {
			return err
		}
		if err := s.recomputeMonitor(ctx, tx, t.ServiceID, svc.MonitorAggregationMode); err != nil {
			return err
		}
		return s.appendServiceStateChanged(ctx, tx, t.ServiceID)
	})
}

func (s *Service) newTarget(serviceID string, in TargetInput, now time.Time) portService.MonitorTarget {
	tt := in.TargetType
	if tt == "" {
		tt = "HTTP"
	}
	ft := in.FailureThreshold
	if ft <= 0 {
		ft = 3
	}
	rt := in.RecoveryThreshold
	if rt <= 0 {
		rt = 2
	}
	im := in.IntervalMS
	if im <= 0 {
		im = 30000
	}
	tm := in.TimeoutMS
	if tm <= 0 {
		tm = 5000
	}
	cfg := in.Config
	if len(cfg) == 0 {
		cfg = []byte("{}")
	}
	return portService.MonitorTarget{
		TargetID: s.deps.IDs.NewID(), ServiceID: serviceID, TargetKey: in.TargetKey,
		TargetName: in.TargetName, Address: in.Address, TargetType: tt, Config: cfg,
		IntervalMS: im, TimeoutMS: tm, FailureThreshold: ft, RecoveryThreshold: rt,
		IsRequired: in.IsRequired, Enabled: true, Status: "UNKNOWN", ConfigGeneration: 1,
	}
}

// recomputeMonitor：目标增删改后重算 MONITOR 来源；APP 来源不受影响。
func (s *Service) recomputeMonitor(ctx context.Context, tx portshared.Tx, serviceID, mode string) error {
	targets, err := s.deps.Targets.ListByService(ctx, tx, serviceID, false)
	if err != nil {
		return err
	}
	status, err := domain.AggregateMonitor(mode, monitorTargetHealths(targets))
	if err != nil {
		return err
	}
	return s.deps.Sources.Upsert(ctx, tx, portService.SourceState{
		ServiceID: serviceID, SourceType: "MONITOR", Status: status,
	})
}

func monitorTargetHealths(targets []portService.MonitorTarget) []domain.TargetHealth {
	out := make([]domain.TargetHealth, 0, len(targets))
	for _, t := range targets {
		out = append(out, domain.TargetHealth{
			Required: t.IsRequired,
			Enabled:  t.Enabled,
			Deleted:  false, // 聚合输入已由 SQL 过滤软删除目标
			Status:   t.Status,
		})
	}
	return out
}

func (s *Service) appendServiceStateChanged(ctx context.Context, tx portshared.Tx, serviceID string) error {
	payload, err := json.Marshal(map[string]string{"service_id": serviceID})
	if err != nil {
		return err
	}
	if err := s.deps.Outbox.Append(ctx, tx, contracts.Event{
		EventID: s.deps.IDs.NewID(), EventType: contracts.EventServiceStateChanged, SchemaVersion: 1,
		AggregateType: "service", AggregateID: serviceID,
		OccurredAt: s.deps.Clock.Now().UTC(), Payload: payload,
	}, nil); err != nil {
		return err
	}
	return s.appendCatalogChanged(ctx, tx, serviceID)
}
