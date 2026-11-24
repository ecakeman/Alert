package postgres

import (
	"context"

	"github.com/ecakeman/alert/internal/application/port"
	"github.com/ecakeman/alert/internal/infrastructure/persistence/postgres/model"
)

type inputVersionRepo struct{}

func NewInputVersionRepo() port.InputVersionRepo { return inputVersionRepo{} }

// AppendIfChanged 以 (episode_id, input_hash) 幂等：同内容不新增，返回 false。
func (inputVersionRepo) AppendIfChanged(ctx context.Context, tx port.Tx, v port.InputVersion) (bool, error) {
	row := model.InputVersionRowFromApp(v)
	tag, err := txFrom(tx).Exec(ctx, `
INSERT INTO system.episode_input_versions(episode_id,input_version,input_hash,monitor_projection,monitor_watermark,app_event_watermark,created_at)
VALUES($1,$2,$3,$4,$5,$6,$7)
ON CONFLICT (episode_id, input_hash) DO NOTHING`,
		row.EpisodeID, row.InputVersion, row.InputHash, row.MonitorProjection,
		row.MonitorWatermark, row.AppEventWatermark, utc(row.CreatedAt))
	if err != nil {
		return false, Translate(err)
	}
	return tag.RowsAffected() == 1, nil
}

func (inputVersionRepo) GetLatest(ctx context.Context, tx port.Tx, episodeID string) (port.InputVersion, error) {
	row, err := model.InputVersionRow{}.Scan(txFrom(tx).QueryRow(ctx,
		`SELECT `+model.InputVersionColumns+` FROM system.episode_input_versions WHERE episode_id=$1 ORDER BY input_version DESC LIMIT 1`,
		episodeID))
	if err != nil {
		return port.InputVersion{}, Translate(err)
	}
	return row.ToApp(), nil
}

func (inputVersionRepo) ListByEpisode(ctx context.Context, tx port.Tx, episodeID string) ([]port.InputVersion, error) {
	rows, err := txFrom(tx).Query(ctx,
		`SELECT `+model.InputVersionColumns+` FROM system.episode_input_versions WHERE episode_id=$1 ORDER BY input_version`,
		episodeID)
	if err != nil {
		return nil, Translate(err)
	}
	defer rows.Close()
	var out []port.InputVersion
	for rows.Next() {
		row, err := model.InputVersionRow{}.Scan(rows)
		if err != nil {
			return nil, Translate(err)
		}
		out = append(out, row.ToApp())
	}
	return out, Translate(rows.Err())
}
