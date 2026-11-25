package model

import (
	"time"

	portService "github.com/ecakeman/alert/services/service-registry/internal/application/port/service"
)

// ServiceColumns 是 system.services 的完整列清单（唯一来源）。
const ServiceColumns = `service_id,service_key,name,description,enabled,monitor_aggregation_mode,authority_producer_id,deleted_at,created_at,updated_at`

// ServiceRow 是 system.services 行模型。
type ServiceRow struct {
	ServiceID              string
	ServiceKey             string
	Name                   string
	Description            string
	Enabled                bool
	MonitorAggregationMode string
	AuthorityProducerID    string
	DeletedAt              *time.Time
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

// Scan 从一行读取 ServiceRow。
func (r ServiceRow) Scan(row Scannable) (ServiceRow, error) {
	if err := row.Scan(&r.ServiceID, &r.ServiceKey, &r.Name, &r.Description, &r.Enabled,
		&r.MonitorAggregationMode, &r.AuthorityProducerID, &r.DeletedAt, &r.CreatedAt, &r.UpdatedAt); err != nil {
		return ServiceRow{}, err
	}
	return r, nil
}

// ToApp 转换为 application 值类型。
func (r ServiceRow) ToApp() portService.Service {
	return portService.Service{
		ServiceID: r.ServiceID, ServiceKey: r.ServiceKey, Name: r.Name,
		Description: r.Description, Enabled: r.Enabled,
		MonitorAggregationMode: r.MonitorAggregationMode,
		AuthorityProducerID:    r.AuthorityProducerID,
	}
}

// ServiceRowFromApp 从 application 值类型构造行模型。
func ServiceRowFromApp(s portService.Service) ServiceRow {
	return ServiceRow{
		ServiceID: s.ServiceID, ServiceKey: s.ServiceKey, Name: s.Name,
		Description: s.Description, Enabled: s.Enabled,
		MonitorAggregationMode: s.MonitorAggregationMode,
		AuthorityProducerID:    s.AuthorityProducerID,
	}
}
