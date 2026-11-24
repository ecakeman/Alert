package model

import (
	"time"

	"github.com/ecakeman/alert/internal/application/port"
)

// EpisodeColumns 是 system.episodes 的完整列清单（唯一来源）。
const EpisodeColumns = `episode_id,service_id,status,started_by,resolution_pending,input_hash,input_version,processed_input_version,final_input_version,monitor_watermark,app_event_watermark,started_at,resolved_at,archived_at,created_at,updated_at`

// EpisodeRow 是 system.episodes 行模型。
type EpisodeRow struct {
	EpisodeID             string
	ServiceID             string
	Status                string
	StartedBy             string
	ResolutionPending     bool
	InputHash             string
	InputVersion          int64
	ProcessedInputVersion int64
	FinalInputVersion     int64
	MonitorWatermark      int64
	AppEventWatermark     int64
	StartedAt             time.Time
	ResolvedAt            *time.Time
	ArchivedAt            *time.Time
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

// Scan 从一行读取 EpisodeRow。
func (r EpisodeRow) Scan(row Scannable) (EpisodeRow, error) {
	if err := row.Scan(&r.EpisodeID, &r.ServiceID, &r.Status, &r.StartedBy, &r.ResolutionPending,
		&r.InputHash, &r.InputVersion, &r.ProcessedInputVersion, &r.FinalInputVersion,
		&r.MonitorWatermark, &r.AppEventWatermark, &r.StartedAt, &r.ResolvedAt, &r.ArchivedAt,
		&r.CreatedAt, &r.UpdatedAt); err != nil {
		return EpisodeRow{}, err
	}
	return r, nil
}

// ToApp 转换为 application 值类型。
func (r EpisodeRow) ToApp() port.Episode {
	return port.Episode{
		EpisodeID: r.EpisodeID, ServiceID: r.ServiceID, Status: r.Status,
		StartedBy: r.StartedBy, ResolutionPending: r.ResolutionPending,
		InputHash: r.InputHash, InputVersion: r.InputVersion,
		ProcessedInputVersion: r.ProcessedInputVersion, FinalInputVersion: r.FinalInputVersion,
		MonitorWatermark: r.MonitorWatermark, AppEventWatermark: r.AppEventWatermark,
		StartedAt: r.StartedAt, ResolvedAt: r.ResolvedAt, ArchivedAt: r.ArchivedAt,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

// EpisodeRowFromApp 从 application 值类型构造行模型。
func EpisodeRowFromApp(e port.Episode) EpisodeRow {
	return EpisodeRow{
		EpisodeID: e.EpisodeID, ServiceID: e.ServiceID, Status: e.Status,
		StartedBy: e.StartedBy, ResolutionPending: e.ResolutionPending,
		InputHash: e.InputHash, InputVersion: e.InputVersion,
		ProcessedInputVersion: e.ProcessedInputVersion, FinalInputVersion: e.FinalInputVersion,
		MonitorWatermark: e.MonitorWatermark, AppEventWatermark: e.AppEventWatermark,
		StartedAt: e.StartedAt, ResolvedAt: e.ResolvedAt, ArchivedAt: e.ArchivedAt,
		CreatedAt: e.CreatedAt, UpdatedAt: e.UpdatedAt,
	}
}
