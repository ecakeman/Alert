package model

import (
	"time"

	portepisode "github.com/ecakeman/alert/internal/application/port/episode"
)

// InputVersionColumns 是 system.episode_input_versions 的完整列清单（唯一来源）。
const InputVersionColumns = `episode_id,input_version,input_hash,monitor_projection,monitor_watermark,app_event_watermark,created_at`

// InputVersionRow 是 system.episode_input_versions 行模型。
type InputVersionRow struct {
	EpisodeID         string
	InputVersion      int64
	InputHash         string
	MonitorProjection []byte
	MonitorWatermark  int64
	AppEventWatermark int64
	CreatedAt         time.Time
}

// Scan 从一行读取 InputVersionRow。
func (r InputVersionRow) Scan(row Scannable) (InputVersionRow, error) {
	if err := row.Scan(&r.EpisodeID, &r.InputVersion, &r.InputHash, &r.MonitorProjection,
		&r.MonitorWatermark, &r.AppEventWatermark, &r.CreatedAt); err != nil {
		return InputVersionRow{}, err
	}
	return r, nil
}

// ToApp 转换为 application 值类型。
func (r InputVersionRow) ToApp() portepisode.InputVersion {
	return portepisode.InputVersion{
		EpisodeID: r.EpisodeID, InputVersion: r.InputVersion, InputHash: r.InputHash,
		MonitorProjection: r.MonitorProjection, MonitorWatermark: r.MonitorWatermark,
		AppEventWatermark: r.AppEventWatermark,
	}
}

// InputVersionRowFromApp 从 application 值类型构造行模型。
func InputVersionRowFromApp(v portepisode.InputVersion) InputVersionRow {
	return InputVersionRow{
		EpisodeID: v.EpisodeID, InputVersion: v.InputVersion, InputHash: v.InputHash,
		MonitorProjection: v.MonitorProjection, MonitorWatermark: v.MonitorWatermark,
		AppEventWatermark: v.AppEventWatermark,
	}
}
