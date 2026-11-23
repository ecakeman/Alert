package postgres

import (
	"context"

	"github.com/ecakeman/alert/internal/application/port"
)

type inputVersionRepo struct{}

func NewInputVersionRepo() port.InputVersionRepo { return inputVersionRepo{} }

const inputVersionColumns = `episode_id,input_version,input_hash,monitor_projection,monitor_watermark,app_event_watermark,created_at`

func scanInputVersion(row scannable) (port.InputVersion, error) {
	var v port.InputVersion
	if err := row.Scan(&v.EpisodeID, &v.InputVersion, &v.InputHash, &v.MonitorProjection,
		&v.MonitorWatermark, &v.AppEventWatermark, &v.CreatedAt); err != nil {
		return port.InputVersion{}, Translate(err)
	}
	return v, nil
}

// AppendIfChanged 以 (episode_id, input_hash) 幂等：同内容不新增，返回 false。
func (inputVersionRepo) AppendIfChanged(ctx context.Context, tx port.Tx, v port.InputVersion) (bool, error) {
	tag, err := txFrom(tx).Exec(ctx, `
INSERT INTO system.episode_input_versions(episode_id,input_version,input_hash,monitor_projection,monitor_watermark,app_event_watermark,created_at)
VALUES($1,$2,$3,$4,$5,$6,$7)
ON CONFLICT (episode_id, input_hash) DO NOTHING`,
		v.EpisodeID, v.InputVersion, v.InputHash, v.MonitorProjection,
		v.MonitorWatermark, v.AppEventWatermark, utc(v.CreatedAt))
	if err != nil {
		return false, Translate(err)
	}
	return tag.RowsAffected() == 1, nil
}

func (inputVersionRepo) GetLatest(ctx context.Context, tx port.Tx, episodeID string) (port.InputVersion, error) {
	return scanInputVersion(txFrom(tx).QueryRow(ctx,
		`SELECT `+inputVersionColumns+` FROM system.episode_input_versions WHERE episode_id=$1 ORDER BY input_version DESC LIMIT 1`,
		episodeID))
}

func (inputVersionRepo) ListByEpisode(ctx context.Context, tx port.Tx, episodeID string) ([]port.InputVersion, error) {
	rows, err := txFrom(tx).Query(ctx,
		`SELECT `+inputVersionColumns+` FROM system.episode_input_versions WHERE episode_id=$1 ORDER BY input_version`,
		episodeID)
	if err != nil {
		return nil, Translate(err)
	}
	defer rows.Close()
	var out []port.InputVersion
	for rows.Next() {
		v, err := scanInputVersion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, Translate(rows.Err())
}
