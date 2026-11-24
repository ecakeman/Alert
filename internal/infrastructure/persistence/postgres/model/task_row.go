package model

import (
	"time"

	"github.com/ecakeman/alert/internal/application/port"
)

// TaskColumns 是 system.agent_tasks 的完整列清单（唯一来源）。
const TaskColumns = `task_id,episode_id,service_id,task_type,status,generation,attempt,max_attempts,lease_until,next_retry_at,input_version_from,input_version_to,monitor_watermark,app_event_watermark,deadline_at,error_code,error_message,created_at,updated_at,started_at,finished_at`

// TaskRow 是 system.agent_tasks 行模型（B7 保留仓储，仅行映射重构）。
type TaskRow struct {
	TaskID            string
	EpisodeID         string
	ServiceID         string
	TaskType          string
	Status            string
	Generation        int64
	Attempt           int
	MaxAttempts       int
	LeaseUntil        *time.Time
	NextRetryAt       *time.Time
	InputVersionFrom  int64
	InputVersionTo    int64
	MonitorWatermark  int64
	AppEventWatermark int64
	DeadlineAt        time.Time
	ErrorCode         string
	ErrorMessage      string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	StartedAt         *time.Time
	FinishedAt        *time.Time
}

// Scan 从一行读取 TaskRow。
func (r TaskRow) Scan(row Scannable) (TaskRow, error) {
	if err := row.Scan(&r.TaskID, &r.EpisodeID, &r.ServiceID, &r.TaskType, &r.Status, &r.Generation,
		&r.Attempt, &r.MaxAttempts, &r.LeaseUntil, &r.NextRetryAt, &r.InputVersionFrom, &r.InputVersionTo,
		&r.MonitorWatermark, &r.AppEventWatermark, &r.DeadlineAt, &r.ErrorCode, &r.ErrorMessage,
		&r.CreatedAt, &r.UpdatedAt, &r.StartedAt, &r.FinishedAt); err != nil {
		return TaskRow{}, err
	}
	return r, nil
}

// ToApp 转换为 application 值类型。
func (r TaskRow) ToApp() port.AgentTask {
	return port.AgentTask{
		TaskID: r.TaskID, EpisodeID: r.EpisodeID, ServiceID: r.ServiceID,
		TaskType: r.TaskType, Status: r.Status, Generation: r.Generation,
		Attempt: r.Attempt, MaxAttempts: r.MaxAttempts,
		LeaseUntil: r.LeaseUntil, NextRetryAt: r.NextRetryAt,
		InputVersionFrom: r.InputVersionFrom, InputVersionTo: r.InputVersionTo,
		MonitorWatermark: r.MonitorWatermark, AppEventWatermark: r.AppEventWatermark,
		DeadlineAt: r.DeadlineAt, ErrorCode: r.ErrorCode, ErrorMessage: r.ErrorMessage,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		StartedAt: r.StartedAt, FinishedAt: r.FinishedAt,
	}
}

// TaskRowFromApp 从 application 值类型构造行模型。
func TaskRowFromApp(t port.AgentTask) TaskRow {
	return TaskRow{
		TaskID: t.TaskID, EpisodeID: t.EpisodeID, ServiceID: t.ServiceID,
		TaskType: t.TaskType, Status: t.Status, Generation: t.Generation,
		Attempt: t.Attempt, MaxAttempts: t.MaxAttempts,
		LeaseUntil: t.LeaseUntil, NextRetryAt: t.NextRetryAt,
		InputVersionFrom: t.InputVersionFrom, InputVersionTo: t.InputVersionTo,
		MonitorWatermark: t.MonitorWatermark, AppEventWatermark: t.AppEventWatermark,
		DeadlineAt: t.DeadlineAt, ErrorCode: t.ErrorCode, ErrorMessage: t.ErrorMessage,
		CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt,
		StartedAt: t.StartedAt, FinishedAt: t.FinishedAt,
	}
}
