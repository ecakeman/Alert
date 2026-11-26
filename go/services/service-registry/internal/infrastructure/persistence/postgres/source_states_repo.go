package postgres

import (
	"context"

	portService "github.com/ecakeman/alert/services/service-registry/internal/application/port/service"
	portshared "github.com/ecakeman/alert/services/service-registry/internal/application/port/shared"
	"github.com/ecakeman/alert/services/service-registry/internal/infrastructure/persistence/postgres/model"
)

type sourceStateRepo struct{}

func NewSourceStateRepo() portService.SourceStateRepo { return sourceStateRepo{} }

func (sourceStateRepo) Get(ctx context.Context, tx portshared.Tx, serviceID, sourceType string) (portService.SourceState, error) {
	row, err := model.SourceStateRow{}.Scan(txFrom(tx).QueryRow(ctx,
		`SELECT `+model.SourceStateColumns+` FROM registry.service_source_states WHERE service_id=$1 AND source_type=$2`,
		serviceID, sourceType))
	if err != nil {
		return portService.SourceState{}, Translate(err)
	}
	return row.ToApp(), nil
}

func (sourceStateRepo) ListByService(ctx context.Context, tx portshared.Tx, serviceID string) ([]portService.SourceState, error) {
	rows, err := txFrom(tx).Query(ctx,
		`SELECT `+model.SourceStateColumns+` FROM registry.service_source_states WHERE service_id=$1 ORDER BY source_type`,
		serviceID)
	if err != nil {
		return nil, Translate(err)
	}
	defer rows.Close()
	var out []portService.SourceState
	for rows.Next() {
		row, err := model.SourceStateRow{}.Scan(rows)
		if err != nil {
			return nil, Translate(err)
		}
		out = append(out, row.ToApp())
	}
	return out, Translate(rows.Err())
}

func (sourceStateRepo) Upsert(ctx context.Context, tx portshared.Tx, s portService.SourceState) error {
	row := model.SourceStateRowFromApp(s)
	_, err := txFrom(tx).Exec(ctx, `
INSERT INTO registry.service_source_states(service_id,source_type,status,reason,updated_at,last_change_at)
VALUES($1,$2,$3,$4,now(),now())
ON CONFLICT (service_id, source_type) DO UPDATE SET
  status = EXCLUDED.status,
  reason = EXCLUDED.reason,
  last_change_at = CASE WHEN registry.service_source_states.status <> EXCLUDED.status THEN now() ELSE registry.service_source_states.last_change_at END,
  updated_at = now()`,
		row.ServiceID, row.SourceType, row.Status, row.Reason)
	return Translate(err)
}
