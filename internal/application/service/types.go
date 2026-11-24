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

// ServiceSummaryPage 是列表读模型：分页元信息 + 过滤后的条目（total 与过滤口径一致）。
type ServiceSummaryPage struct {
	port.Page
	Items []ServiceSummaryData
}
