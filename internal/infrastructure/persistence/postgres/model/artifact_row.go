package model

import (
	"time"

	porttask "github.com/ecakeman/alert/internal/application/port/task"
)

// ArtifactColumns 是 system.episode_artifact_refs 的完整列清单（唯一来源）。
const ArtifactColumns = `artifact_ref_id,episode_id,task_id,generation,artifact_type,artifact_version,content_hash,content_type,storage_key,source_refs,registered_at,updated_at`

// ArtifactRow 是 system.episode_artifact_refs 行模型（B7 保留仓储）。
type ArtifactRow struct {
	ArtifactRefID   string
	EpisodeID       string
	TaskID          string
	Generation      int64
	ArtifactType    string
	ArtifactVersion int
	ContentHash     string
	ContentType     string
	StorageKey      string
	SourceRefs      []byte
	RegisteredAt    time.Time
	UpdatedAt       time.Time
}

// Scan 从一行读取 ArtifactRow。
func (r ArtifactRow) Scan(row Scannable) (ArtifactRow, error) {
	if err := row.Scan(&r.ArtifactRefID, &r.EpisodeID, &r.TaskID, &r.Generation, &r.ArtifactType,
		&r.ArtifactVersion, &r.ContentHash, &r.ContentType, &r.StorageKey, &r.SourceRefs,
		&r.RegisteredAt, &r.UpdatedAt); err != nil {
		return ArtifactRow{}, err
	}
	return r, nil
}

// ToApp 转换为 application 值类型。
func (r ArtifactRow) ToApp() porttask.ArtifactRef {
	return porttask.ArtifactRef{
		ArtifactRefID: r.ArtifactRefID, EpisodeID: r.EpisodeID, TaskID: r.TaskID,
		Generation: r.Generation, ArtifactType: r.ArtifactType, ArtifactVersion: r.ArtifactVersion,
		ContentHash: r.ContentHash, ContentType: r.ContentType, StorageKey: r.StorageKey,
		SourceRefs: r.SourceRefs, RegisteredAt: r.RegisteredAt, UpdatedAt: r.UpdatedAt,
	}
}

// ArtifactRowFromApp 从 application 值类型构造行模型。
func ArtifactRowFromApp(a porttask.ArtifactRef) ArtifactRow {
	return ArtifactRow{
		ArtifactRefID: a.ArtifactRefID, EpisodeID: a.EpisodeID, TaskID: a.TaskID,
		Generation: a.Generation, ArtifactType: a.ArtifactType, ArtifactVersion: a.ArtifactVersion,
		ContentHash: a.ContentHash, ContentType: a.ContentType, StorageKey: a.StorageKey,
		SourceRefs: a.SourceRefs, RegisteredAt: a.RegisteredAt, UpdatedAt: a.UpdatedAt,
	}
}
