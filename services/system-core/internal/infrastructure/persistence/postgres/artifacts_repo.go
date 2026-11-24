package postgres

import (
	"context"

	portshared "github.com/ecakeman/alert/services/system-core/internal/application/port/shared"
	porttask "github.com/ecakeman/alert/services/system-core/internal/application/port/task"
	"github.com/ecakeman/alert/services/system-core/internal/infrastructure/persistence/postgres/model"
)

type artifactRepo struct{}

func NewArtifactRepo() porttask.ArtifactRepo { return artifactRepo{} }

func (artifactRepo) GetLatest(ctx context.Context, tx portshared.Tx, episodeID, artifactType string) (porttask.ArtifactRef, error) {
	row, err := model.ArtifactRow{}.Scan(txFrom(tx).QueryRow(ctx, `
SELECT `+model.ArtifactColumns+` FROM system.episode_artifact_refs
WHERE episode_id=$1 AND artifact_type=$2
ORDER BY artifact_version DESC LIMIT 1`, episodeID, artifactType))
	if err != nil {
		return porttask.ArtifactRef{}, Translate(err)
	}
	return row.ToApp(), nil
}

func (artifactRepo) ListByEpisode(ctx context.Context, tx portshared.Tx, episodeID string) ([]porttask.ArtifactRef, error) {
	rows, err := txFrom(tx).Query(ctx, `
SELECT `+model.ArtifactColumns+` FROM system.episode_artifact_refs
WHERE episode_id=$1 ORDER BY artifact_type, artifact_version DESC`, episodeID)
	if err != nil {
		return nil, Translate(err)
	}
	defer rows.Close()
	var out []porttask.ArtifactRef
	for rows.Next() {
		row, err := model.ArtifactRow{}.Scan(rows)
		if err != nil {
			return nil, Translate(err)
		}
		out = append(out, row.ToApp())
	}
	return out, Translate(rows.Err())
}

// GetByTypeVersion 读取同故障同类型同版本产物，供幂等 / 冲突判定。
func (artifactRepo) GetByTypeVersion(ctx context.Context, tx portshared.Tx, episodeID, artifactType string, version int) (porttask.ArtifactRef, error) {
	row, err := model.ArtifactRow{}.Scan(txFrom(tx).QueryRow(ctx, `
SELECT `+model.ArtifactColumns+` FROM system.episode_artifact_refs
WHERE episode_id=$1 AND artifact_type=$2 AND artifact_version=$3`, episodeID, artifactType, version))
	if err != nil {
		return porttask.ArtifactRef{}, Translate(err)
	}
	return row.ToApp(), nil
}

func (artifactRepo) Insert(ctx context.Context, tx portshared.Tx, a porttask.ArtifactRef) error {
	row := model.ArtifactRowFromApp(a)
	sourceRefsJSON := string(row.SourceRefs)
	if sourceRefsJSON == "" {
		sourceRefsJSON = "[]"
	}
	_, err := txFrom(tx).Exec(ctx, `
INSERT INTO system.episode_artifact_refs(artifact_ref_id,episode_id,task_id,generation,artifact_type,artifact_version,content_hash,content_type,storage_key,source_refs,registered_at,updated_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		row.ArtifactRefID, row.EpisodeID, row.TaskID, row.Generation, row.ArtifactType,
		row.ArtifactVersion, row.ContentHash, row.ContentType, row.StorageKey,
		sourceRefsJSON, utc(row.RegisteredAt), utc(row.UpdatedAt))
	return Translate(err)
}
