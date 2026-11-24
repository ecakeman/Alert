package postgres

import (
	"context"
	"time"

	"github.com/ecakeman/alert/internal/application/port"
	"github.com/ecakeman/alert/internal/infrastructure/persistence/postgres/model"
)

type episodeRepo struct{}

func NewEpisodeRepo() port.EpisodeRepo { return episodeRepo{} }

func (episodeRepo) GetActiveByService(ctx context.Context, tx port.Tx, serviceID string) (port.Episode, error) {
	row, err := model.EpisodeRow{}.Scan(txFrom(tx).QueryRow(ctx,
		`SELECT `+model.EpisodeColumns+` FROM system.episodes WHERE service_id=$1 AND status='ACTIVE' FOR UPDATE`, serviceID))
	if err != nil {
		return port.Episode{}, Translate(err)
	}
	return row.ToApp(), nil
}

func (episodeRepo) Get(ctx context.Context, tx port.Tx, episodeID string) (port.Episode, error) {
	row, err := model.EpisodeRow{}.Scan(txFrom(tx).QueryRow(ctx,
		`SELECT `+model.EpisodeColumns+` FROM system.episodes WHERE episode_id=$1`, episodeID))
	if err != nil {
		return port.Episode{}, Translate(err)
	}
	return row.ToApp(), nil
}

func (episodeRepo) List(ctx context.Context, tx port.Tx, f port.EpisodeFilter) ([]port.Episode, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = 20
	}
	rows, err := txFrom(tx).Query(ctx, `
SELECT `+model.EpisodeColumns+` FROM system.episodes
WHERE ($1 = '' OR service_id::text = $1)
  AND ($2 = '' OR status = $2)
  AND ($3::timestamptz IS NULL OR started_at >= $3)
  AND ($4::timestamptz IS NULL OR started_at <= $4)
ORDER BY started_at DESC
LIMIT $5 OFFSET $6`, f.ServiceID, f.Status, f.From, f.To, limit, f.Offset)
	if err != nil {
		return nil, Translate(err)
	}
	defer rows.Close()
	var out []port.Episode
	for rows.Next() {
		row, err := model.EpisodeRow{}.Scan(rows)
		if err != nil {
			return nil, Translate(err)
		}
		out = append(out, row.ToApp())
	}
	return out, Translate(rows.Err())
}

func (episodeRepo) Create(ctx context.Context, tx port.Tx, e port.Episode) error {
	row := model.EpisodeRowFromApp(e)
	_, err := txFrom(tx).Exec(ctx, `
INSERT INTO system.episodes(episode_id,service_id,status,started_by,resolution_pending,input_hash,input_version,processed_input_version,final_input_version,monitor_watermark,app_event_watermark,started_at,created_at,updated_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
		row.EpisodeID, row.ServiceID, row.Status, row.StartedBy, row.ResolutionPending,
		row.InputHash, row.InputVersion, row.ProcessedInputVersion, row.FinalInputVersion,
		row.MonitorWatermark, row.AppEventWatermark, utc(row.StartedAt), utc(row.CreatedAt), utc(row.UpdatedAt))
	return Translate(err)
}

func (episodeRepo) UpdateStatus(ctx context.Context, tx port.Tx, episodeID, status string, at time.Time, resolvedAt, archivedAt *time.Time) error {
	_, err := txFrom(tx).Exec(ctx, `
UPDATE system.episodes SET
  status = $2,
  resolved_at = COALESCE($4, resolved_at),
  archived_at = COALESCE($5, archived_at),
  updated_at = $3
WHERE episode_id=$1`, episodeID, status, utc(at), utcPtr(resolvedAt), utcPtr(archivedAt))
	return Translate(err)
}

func (episodeRepo) TouchInputVersion(ctx context.Context, tx port.Tx, episodeID string, inputVersion int64, hash string, monitorWatermark, appWatermark int64) error {
	_, err := txFrom(tx).Exec(ctx, `
UPDATE system.episodes SET
  input_version = $2,
  input_hash = $3,
  monitor_watermark = $4,
  app_event_watermark = $5,
  updated_at = now()
WHERE episode_id=$1`, episodeID, inputVersion, hash, monitorWatermark, appWatermark)
	return Translate(err)
}
