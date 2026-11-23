package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/ecakeman/alert/internal/application/apperr"
	"github.com/ecakeman/alert/internal/application/port"
)

// Deps 是用例依赖的端口集合。
type Deps struct {
	UOW      port.UnitOfWork
	Services port.ServiceRepo
	Targets  port.TargetRepo
	Sources  port.SourceStateRepo
	Episodes port.EpisodeRepo
	Outbox   port.OutboxRepo
	Clock    port.Clock
	IDs      port.IDGen
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

	var view ServiceView
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx port.Tx) error {
		if _, err := s.deps.Services.GetByKey(ctx, tx, in.ServiceKey); err == nil {
			return apperr.AlreadyExists("service_key already exists")
		} else if !isAppCode(err, apperr.CodeNotFound) {
			return err
		}

		svcID := s.deps.IDs.NewID()
		now := s.deps.Clock.Now().UTC()
		svc := port.Service{
			ServiceID: svcID, ServiceKey: in.ServiceKey, Name: in.Name, Description: in.Description,
			Enabled: true, MonitorAggregationMode: mode, AuthorityProducerID: in.AuthorityProducerID,
			CreatedAt: now, UpdatedAt: now,
		}
		if err := s.deps.Services.Create(ctx, tx, svc); err != nil {
			return err
		}
		for _, st := range []string{"MONITOR", "APP"} {
			if err := s.deps.Sources.Upsert(ctx, tx, port.SourceState{
				ServiceID: svcID, SourceType: st, Status: "UNKNOWN",
			}); err != nil {
				return err
			}
		}

		var targets []port.MonitorTarget
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

func (s *Service) List(ctx context.Context, f port.ServiceFilter) ([]port.Service, error) {
	var out []port.Service
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx port.Tx) error {
		var err error
		out, err = s.deps.Services.List(ctx, tx, f)
		return err
	})
	return out, err
}

func (s *Service) Get(ctx context.Context, serviceID string) (ServiceView, error) {
	var view ServiceView
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx port.Tx) error {
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

func (s *Service) Patch(ctx context.Context, serviceID string, p port.ServicePatch) (ServiceView, error) {
	if p.Name == nil && p.Description == nil && p.Enabled == nil && p.MonitorAggregationMode == nil {
		return ServiceView{}, apperr.Validation("no updatable fields")
	}
	var view ServiceView
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx port.Tx) error {
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
	return s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx port.Tx) error {
		if _, err := s.deps.Services.Get(ctx, tx, serviceID); err != nil {
			return err
		}
		if err := s.deps.Services.SoftDelete(ctx, tx, serviceID, s.deps.Clock.Now().UTC()); err != nil {
			return err
		}
		return s.appendServiceStateChanged(ctx, tx, serviceID)
	})
}

func (s *Service) ListTargets(ctx context.Context, serviceID string) ([]port.MonitorTarget, error) {
	var out []port.MonitorTarget
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx port.Tx) error {
		if _, err := s.deps.Services.Get(ctx, tx, serviceID); err != nil {
			return err
		}
		var err error
		out, err = s.deps.Targets.ListByService(ctx, tx, serviceID, false)
		return err
	})
	return out, err
}

func (s *Service) CreateTarget(ctx context.Context, serviceID string, in TargetInput) (port.MonitorTarget, error) {
	var t port.MonitorTarget
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx port.Tx) error {
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

func (s *Service) PatchTarget(ctx context.Context, targetID string, p port.TargetPatch) (port.MonitorTarget, error) {
	var t port.MonitorTarget
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx port.Tx) error {
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
	return s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx port.Tx) error {
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

func (s *Service) newTarget(serviceID string, in TargetInput, now time.Time) port.MonitorTarget {
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
	return port.MonitorTarget{
		TargetID: s.deps.IDs.NewID(), ServiceID: serviceID, TargetKey: in.TargetKey,
		TargetName: in.TargetName, Address: in.Address, TargetType: tt, Config: in.Config,
		IntervalMS: im, TimeoutMS: tm, FailureThreshold: ft, RecoveryThreshold: rt,
		IsRequired: in.IsRequired, Enabled: true, Status: "UNKNOWN", ConfigGeneration: 1,
		CreatedAt: now, UpdatedAt: now,
	}
}

// recomputeMonitor：目标增删改后重算 MONITOR 来源；APP 来源不受影响。
func (s *Service) recomputeMonitor(ctx context.Context, tx port.Tx, serviceID, mode string) error {
	targets, err := s.deps.Targets.ListByService(ctx, tx, serviceID, false)
	if err != nil {
		return err
	}
	var required []port.MonitorTarget
	for _, t := range targets {
		if t.IsRequired {
			required = append(required, t)
		}
	}

	status := "UNKNOWN"
	if len(required) > 0 {
		normal, abnormal := 0, 0
		for _, t := range required {
			switch t.Status {
			case "NORMAL":
				normal++
			case "ABNORMAL":
				abnormal++
			}
		}
		switch mode {
		case "ANY_REQUIRED":
			if abnormal > 0 {
				status = "ABNORMAL"
			} else if normal > 0 {
				status = "NORMAL"
			}
		default: // ALL_REQUIRED / MAJORITY 简化按 ALL_REQUIRED
			if abnormal > 0 {
				status = "ABNORMAL"
			} else if normal == len(required) {
				status = "NORMAL"
			}
		}
	}

	return s.deps.Sources.Upsert(ctx, tx, port.SourceState{
		ServiceID: serviceID, SourceType: "MONITOR", Status: status,
	})
}

func (s *Service) appendServiceStateChanged(ctx context.Context, tx port.Tx, serviceID string) error {
	payload, err := json.Marshal(map[string]string{"service_id": serviceID})
	if err != nil {
		return err
	}
	return s.deps.Outbox.Append(ctx, tx, port.OutboxEvent{
		EventID: s.deps.IDs.NewID(), EventType: "service.state.changed.v1", SchemaVersion: 1,
		AggregateType: "service", AggregateID: serviceID, Payload: payload,
		CreatedAt: s.deps.Clock.Now().UTC(),
	})
}
