package model

import (
	"time"

	porttask "github.com/ecakeman/alert/services/system-core/internal/application/port/task"
)

// OwnershipColumns 是 system.service_execution_ownership 的完整列清单（唯一来源）。
const OwnershipColumns = `service_id,task_id,episode_id,task_type,generation,lease_until,acquired_at,updated_at`

// OwnershipRow 是 system.service_execution_ownership 行模型（B7 保留仓储）。
type OwnershipRow struct {
	ServiceID  string
	TaskID     string
	EpisodeID  string
	TaskType   string
	Generation int64
	LeaseUntil time.Time
	AcquiredAt time.Time
	UpdatedAt  time.Time
}

// Scan 从一行读取 OwnershipRow。
func (r OwnershipRow) Scan(row Scannable) (OwnershipRow, error) {
	if err := row.Scan(&r.ServiceID, &r.TaskID, &r.EpisodeID, &r.TaskType, &r.Generation,
		&r.LeaseUntil, &r.AcquiredAt, &r.UpdatedAt); err != nil {
		return OwnershipRow{}, err
	}
	return r, nil
}

// ToApp 转换为 application 值类型。
func (r OwnershipRow) ToApp() porttask.Ownership {
	return porttask.Ownership{
		ServiceID: r.ServiceID, TaskID: r.TaskID, EpisodeID: r.EpisodeID,
		TaskType: r.TaskType, Generation: r.Generation,
		LeaseUntil: r.LeaseUntil, AcquiredAt: r.AcquiredAt, UpdatedAt: r.UpdatedAt,
	}
}
