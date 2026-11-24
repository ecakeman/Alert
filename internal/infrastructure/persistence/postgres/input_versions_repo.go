package postgres

import (
	"context"

	portepisode "github.com/ecakeman/alert/internal/application/port/episode"
	portshared "github.com/ecakeman/alert/internal/application/port/shared"
	"github.com/ecakeman/alert/internal/infrastructure/persistence/postgres/model"
)

type inputVersionRepo struct{}

func NewInputVersionRepo() portepisode.InputVersionRepo { return inputVersionRepo{} }

// AppendIfChanged 以 (episode_id, input_hash) 幂等：同内容不新增，返回 false。
// created_at 由 DB DEFAULT now() 接管。
func (inputVersionRepo) AppendIfChanged(ctx context.Context, tx portshared.Tx, v portepisode.InputVersion) (bool, error) {
	row := model.InputVersionRowFromApp(v)
	tag, err := txFrom(tx).Exec(ctx, `
INSERT INTO system.episode_input_versions(episode_id,input_version,input_hash,monitor_projection,monitor_watermark,app_event_watermark)
VALUES($1,$2,$3,$4,$5,$6)
ON CONFLICT (episode_id, input_hash) DO NOTHING`,
		row.EpisodeID, row.InputVersion, row.InputHash, row.MonitorProjection,
		row.MonitorWatermark, row.AppEventWatermark)
	if err != nil {
		return false, Translate(err)
	}
	return tag.RowsAffected() == 1, nil
}

func (inputVersionRepo) GetLatest(ctx context.Context, tx portshared.Tx, episodeID string) (portepisode.InputVersion, error) {
	row, err := model.InputVersionRow{}.Scan(txFrom(tx).QueryRow(ctx,
		`SELECT `+model.InputVersionColumns+` FROM system.episode_input_versions WHERE episode_id=$1 ORDER BY input_version DESC LIMIT 1`,
		episodeID))
	if err != nil {
		return portepisode.InputVersion{}, Translate(err)
	}
	return row.ToApp(), nil
}

func (inputVersionRepo) ListByEpisode(ctx context.Context, tx portshared.Tx, episodeID string) ([]portepisode.InputVersion, error) {
	rows, err := txFrom(tx).Query(ctx,
		`SELECT `+model.InputVersionColumns+` FROM system.episode_input_versions WHERE episode_id=$1 ORDER BY input_version`,
		episodeID)
	if err != nil {
		return nil, Translate(err)
	}
	defer rows.Close()
	var out []portepisode.InputVersion
	for rows.Next() {
		row, err := model.InputVersionRow{}.Scan(rows)
		if err != nil {
			return nil, Translate(err)
		}
		out = append(out, row.ToApp())
	}
	return out, Translate(rows.Err())
}
