package postgres

import (
	"context"
	"time"

	portepisode "github.com/ecakeman/alert/services/episode-svc/internal/application/port/episode"
	portshared "github.com/ecakeman/alert/services/episode-svc/internal/application/port/shared"
	"github.com/ecakeman/alert/services/episode-svc/internal/infrastructure/persistence/postgres/model"
)

type episodeRepo struct{}

func NewEpisodeRepo() portepisode.EpisodeRepo { return episodeRepo{} }

func (episodeRepo) GetActiveByService(ctx context.Context, tx portshared.Tx, serviceID string) (portepisode.Episode, error) {
	row, err := model.EpisodeRow{}.Scan(txFrom(tx).QueryRow(ctx,
		`SELECT `+model.EpisodeColumns+` FROM episode.episodes WHERE service_id=$1 AND status='ACTIVE' FOR UPDATE`, serviceID))
	if err != nil {
		return portepisode.Episode{}, Translate(err)
	}
	return row.ToApp(), nil
}

func (episodeRepo) Get(ctx context.Context, tx portshared.Tx, episodeID string) (portepisode.Episode, error) {
	row, err := model.EpisodeRow{}.Scan(txFrom(tx).QueryRow(ctx,
		`SELECT `+model.EpisodeColumns+` FROM episode.episodes WHERE episode_id=$1`, episodeID))
	if err != nil {
		return portepisode.Episode{}, Translate(err)
	}
	return row.ToApp(), nil
}

func (episodeRepo) List(ctx context.Context, tx portshared.Tx, f portepisode.EpisodeFilter) ([]portepisode.Episode, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = 20
	}
	rows, err := txFrom(tx).Query(ctx, `
SELECT `+model.EpisodeColumns+` FROM episode.episodes
WHERE ($1 = '' OR service_id::text = $1)
  AND ($2 = '' OR status = $2)
ORDER BY started_at DESC
LIMIT $3 OFFSET $4`, f.ServiceID, f.Status, limit, f.Offset)
	if err != nil {
		return nil, Translate(err)
	}
	defer rows.Close()
	var out []portepisode.Episode
	for rows.Next() {
		row, err := model.EpisodeRow{}.Scan(rows)
		if err != nil {
			return nil, Translate(err)
		}
		out = append(out, row.ToApp())
	}
	return out, Translate(rows.Err())
}

// Create：created_at/updated_at 由 DB DEFAULT now() 接管；started_at 显式传入。
func (episodeRepo) Create(ctx context.Context, tx portshared.Tx, e portepisode.Episode) error {
	row := model.EpisodeRowFromApp(e)
	_, err := txFrom(tx).Exec(ctx, `
INSERT INTO episode.episodes(episode_id,service_id,status,started_by,resolution_pending,input_hash,input_version,processed_input_version,final_input_version,monitor_watermark,app_event_watermark,started_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		row.EpisodeID, row.ServiceID, row.Status, row.StartedBy, row.ResolutionPending,
		row.InputHash, row.InputVersion, row.ProcessedInputVersion, row.FinalInputVersion,
		row.MonitorWatermark, row.AppEventWatermark, utc(row.StartedAt))
	return Translate(err)
}

func (episodeRepo) UpdateStatus(ctx context.Context, tx portshared.Tx, episodeID, status string, at time.Time, resolvedAt, archivedAt *time.Time) error {
	_, err := txFrom(tx).Exec(ctx, `
UPDATE episode.episodes SET
  status = $2,
  resolved_at = COALESCE($4, resolved_at),
  archived_at = COALESCE($5, archived_at),
  updated_at = $3
WHERE episode_id=$1`, episodeID, status, utc(at), utcPtr(resolvedAt), utcPtr(archivedAt))
	return Translate(err)
}

// ResolveIfUnchanged 以 CAS 方式解除：仅当仍 ACTIVE 且 updated_at 未被并发修改时生效。
// 该列的时间精度必须与写入值同源（同一 Clock，UTC），禁止在 SQL 内 now() 重算。
func (episodeRepo) ResolveIfUnchanged(ctx context.Context, tx portshared.Tx, episodeID string, expectedUpdatedAt, resolvedAt time.Time) (bool, error) {
	tag, err := txFrom(tx).Exec(ctx, `
UPDATE episode.episodes SET status='RESOLVED', resolved_at=$3, updated_at=$3
WHERE episode_id=$1 AND status='ACTIVE' AND updated_at=$2`,
		episodeID, utc(expectedUpdatedAt), utc(resolvedAt))
	if err != nil {
		return false, Translate(err)
	}
	return tag.RowsAffected() > 0, nil
}

func (episodeRepo) Count(ctx context.Context, tx portshared.Tx, f portepisode.EpisodeFilter) (int, error) {
	var total int
	err := txFrom(tx).QueryRow(ctx, `
SELECT count(*) FROM episode.episodes
WHERE ($1 = '' OR service_id::text = $1)
  AND ($2 = '' OR status = $2)`, f.ServiceID, f.Status).Scan(&total)
	return total, Translate(err)
}

func (episodeRepo) ListActiveByServices(ctx context.Context, tx portshared.Tx, serviceIDs []string) ([]portepisode.Episode, error) {
	rows, err := txFrom(tx).Query(ctx, `
SELECT `+model.EpisodeColumns+` FROM episode.episodes
WHERE status='ACTIVE' AND service_id::text = ANY($1)
ORDER BY started_at DESC`, serviceIDs)
	if err != nil {
		return nil, Translate(err)
	}
	defer rows.Close()
	var out []portepisode.Episode
	for rows.Next() {
		row, err := model.EpisodeRow{}.Scan(rows)
		if err != nil {
			return nil, Translate(err)
		}
		out = append(out, row.ToApp())
	}
	return out, Translate(rows.Err())
}

func (episodeRepo) TouchInputVersion(ctx context.Context, tx portshared.Tx, episodeID string, inputVersion int64, hash string, monitorWatermark, appWatermark int64) error {
	_, err := txFrom(tx).Exec(ctx, `
UPDATE episode.episodes SET
  input_version = $2,
  input_hash = $3,
  monitor_watermark = $4,
  app_event_watermark = $5,
  updated_at = now()
WHERE episode_id=$1`, episodeID, inputVersion, hash, monitorWatermark, appWatermark)
	return Translate(err)
}