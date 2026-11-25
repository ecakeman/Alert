package service

import (
	portService "github.com/ecakeman/alert/services/service-registry/internal/application/port/service"
)

type TargetInput struct {
	TargetKey         string
	TargetName        string
	Address           string
	TargetType        string
	Config            []byte
	IntervalMS        int
	TimeoutMS         int
	FailureThreshold  int
	RecoveryThreshold int
	IsRequired        bool
}

type CreateServiceInput struct {
	ServiceKey             string
	Name                   string
	Description            string
	MonitorAggregationMode string
	AuthorityProducerID    string
	Targets                []TargetInput
}

type ServiceView struct {
	Service portService.Service
	Targets []portService.MonitorTarget
}
