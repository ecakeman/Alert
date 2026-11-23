package service

import "github.com/ecakeman/alert/internal/application/port"

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
	Service port.Service
	Targets []port.MonitorTarget
}
