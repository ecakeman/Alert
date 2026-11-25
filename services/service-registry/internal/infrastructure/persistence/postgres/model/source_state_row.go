package model

import (
	"time"

	portService "github.com/ecakeman/alert/services/service-registry/internal/application/port/service"
)

// SourceStateColumns 是 system.service_source_states 的完整列清单（唯一来源）。
const SourceStateColumns = `service_id,source_type,status,reason,updated_at,last_change_at`

// SourceStateRow 是 system.service_source_states 行模型（PK：service_id + source_type）。
type SourceStateRow struct {
	ServiceID    string
	SourceType   string
	Status       string
	Reason       string
	UpdatedAt    time.Time
	LastChangeAt time.Time
}

// Scan 从一行读取 SourceStateRow。
func (r SourceStateRow) Scan(row Scannable) (SourceStateRow, error) {
	if err := row.Scan(&r.ServiceID, &r.SourceType, &r.Status, &r.Reason, &r.UpdatedAt, &r.LastChangeAt); err != nil {
		return SourceStateRow{}, err
	}
	return r, nil
}

// ToApp 转换为 application 值类型。
func (r SourceStateRow) ToApp() portService.SourceState {
	return portService.SourceState{
		ServiceID: r.ServiceID, SourceType: r.SourceType, Status: r.Status,
		Reason: r.Reason,
	}
}

// SourceStateRowFromApp 从 application 值类型构造行模型。
func SourceStateRowFromApp(s portService.SourceState) SourceStateRow {
	return SourceStateRow{
		ServiceID: s.ServiceID, SourceType: s.SourceType, Status: s.Status,
		Reason: s.Reason,
	}
}
