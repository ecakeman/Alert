package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ecakeman/alert/services/service-registry/internal/application/apperr"
	portshared "github.com/ecakeman/alert/services/service-registry/internal/application/port/shared"
	"github.com/ecakeman/alert/services/service-registry/internal/contracts"
)

type catalogService struct {
	ID              string `json:"id"`
	Enabled         bool   `json:"enabled"`
	AggregationMode string `json:"aggregation_mode"`
}

type catalogTarget struct {
	TargetID          string `json:"target_id"`
	TargetKey         string `json:"target_key"`
	TargetType        string `json:"target_type"`
	Address           string `json:"address"`
	IntervalMS        int    `json:"interval_ms"`
	TimeoutMS         int    `json:"timeout_ms"`
	FailureThreshold  int    `json:"failure_threshold"`
	RecoveryThreshold int    `json:"recovery_threshold"`
	IsRequired        bool   `json:"is_required"`
	Enabled           bool   `json:"enabled"`
	ConfigGeneration  int64  `json:"config_generation"`
}

type catalogPayload struct {
	Service catalogService  `json:"service"`
	Targets []catalogTarget `json:"targets"`
}

// appendCatalogChanged 在写路径同一事务内发布完整配置快照。
// 软删后的 Get 走 not-found 分支，发 enabled=false + 空 targets 的移除快照。
func (s *Service) appendCatalogChanged(ctx context.Context, tx portshared.Tx, serviceID string) error {
	payload := catalogPayload{Targets: []catalogTarget{}}
	svc, err := s.deps.Services.Get(ctx, tx, serviceID)
	if err != nil {
		if !isNotFound(err) {
			return err
		}
		payload.Service = catalogService{ID: serviceID, Enabled: false}
	} else {
		targets, err := s.deps.Targets.ListByService(ctx, tx, serviceID, false)
		if err != nil {
			return err
		}
		payload.Service = catalogService{
			ID: svc.ServiceID, Enabled: svc.Enabled, AggregationMode: svc.MonitorAggregationMode,
		}
		for _, target := range targets {
			payload.Targets = append(payload.Targets, catalogTarget{
				TargetID: target.TargetID, TargetKey: target.TargetKey, TargetType: target.TargetType,
				Address: target.Address, IntervalMS: target.IntervalMS, TimeoutMS: target.TimeoutMS,
				FailureThreshold: target.FailureThreshold, RecoveryThreshold: target.RecoveryThreshold,
				IsRequired: target.IsRequired, Enabled: target.Enabled,
				ConfigGeneration: target.ConfigGeneration,
			})
		}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	dedupe := fmt.Sprintf("catalog:%s:%s", serviceID, s.deps.IDs.NewID())
	return s.deps.Outbox.Append(ctx, tx, contracts.Event{
		EventID: s.deps.IDs.NewID(), EventType: contracts.EventServiceCatalogChanged, SchemaVersion: 1,
		AggregateType: "service", AggregateID: serviceID,
		OccurredAt: s.deps.Clock.Now().UTC(), Payload: body,
	}, &dedupe)
}

func isNotFound(err error) bool {
	var ae *apperr.Error
	return errors.As(err, &ae) && ae.Code == apperr.CodeNotFound
}
