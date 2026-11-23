package postgres

import (
	"context"

	"github.com/ecakeman/alert/internal/application/port"
)

type artifactRepo struct{}

func NewArtifactRepo() port.ArtifactRepo { return artifactRepo{} }

const artifactColumns = `artifact_ref_id,episode_id,task_id,generation,artifact_type,artifact_version,content_hash,content_type,storage_key,source_refs,registered_at,updated_at`

func scanArtifact(row scannable) (port.ArtifactRef, error) {
	var a port.ArtifactRef
	if err := row.Scan(&a.ArtifactRefID, &a.EpisodeID, &a.TaskID, &a.Generation, &a.ArtifactType,
		&a.ArtifactVersion, &a.ContentHash, &a.ContentType, &a.StorageKey, &a.SourceRefs,
		&a.RegisteredAt, &a.UpdatedAt); err != nil {
		return port.ArtifactRef{}, Translate(err)
	}
	return a, nil
}

func (artifactRepo) GetLatest(ctx context.Context, tx port.Tx, episodeID, artifactType string) (port.ArtifactRef, error) {
	return scanArtifact(txFrom(tx).QueryRow(ctx, `
SELECT `+artifactColumns+` FROM system.episode_artifact_refs
WHERE episode_id=$1 AND artifact_type=$2
ORDER BY artifact_version DESC LIMIT 1`, episodeID, artifactType))
}

func (artifactRepo) ListByEpisode(ctx context.Context, tx port.Tx, episodeID string) ([]port.ArtifactRef, error) {
	rows, err := txFrom(tx).Query(ctx, `
SELECT `+artifactColumns+` FROM system.episode_artifact_refs
WHERE episode_id=$1 ORDER BY artifact_type, artifact_version DESC`, episodeID)
	if err != nil {
		return nil, Translate(err)
	}
	defer rows.Close()
	var out []port.ArtifactRef
	for rows.Next() {
		a, err := scanArtifact(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, Translate(rows.Err())
}

func (artifactRepo) Insert(ctx context.Context, tx port.Tx, a port.ArtifactRef) error {
	_, err := txFrom(tx).Exec(ctx, `
INSERT INTO system.episode_artifact_refs(artifact_ref_id,episode_id,task_id,generation,artifact_type,artifact_version,content_hash,content_type,storage_key,source_refs,registered_at,updated_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		a.ArtifactRefID, a.EpisodeID, a.TaskID, a.Generation, a.ArtifactType, a.ArtifactVersion,
		a.ContentHash, a.ContentType, a.StorageKey, a.SourceRefs, utc(a.RegisteredAt), utc(a.UpdatedAt))
	return Translate(err)
}