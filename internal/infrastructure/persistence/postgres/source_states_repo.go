package postgres

import (
	"context"

	"github.com/ecakeman/alert/internal/application/port"
)

type sourceStateRepo struct{}

func NewSourceStateRepo() port.SourceStateRepo { return sourceStateRepo{} }

const sourceStateColumns = `service_id,source_type,status,reason,updated_at,last_change_at`

func scanSourceState(row scannable) (port.SourceState, error) {
	var s port.SourceState
	if err := row.Scan(&s.ServiceID, &s.SourceType, &s.Status, &s.Reason, &s.UpdatedAt, &s.LastChangeAt); err != nil {
		return port.SourceState{}, Translate(err)
	}
	return s, nil
}

func (sourceStateRepo) Get(ctx context.Context, tx port.Tx, serviceID, sourceType string) (port.SourceState, error) {
	return scanSourceState(txFrom(tx).QueryRow(ctx,
		`SELECT `+sourceStateColumns+` FROM system.service_source_states WHERE service_id=$1 AND source_type=$2`,
		serviceID, sourceType))
}

func (sourceStateRepo) ListByService(ctx context.Context, tx port.Tx, serviceID string) ([]port.SourceState, error) {
	rows, err := txFrom(tx).Query(ctx,
		`SELECT `+sourceStateColumns+` FROM system.service_source_states WHERE service_id=$1 ORDER BY source_type`,
		serviceID)
	if err != nil {
		return nil, Translate(err)
	}
	defer rows.Close()
	var out []port.SourceState
	for rows.Next() {
		s, err := scanSourceState(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, Translate(rows.Err())
}

func (sourceStateRepo) Upsert(ctx context.Context, tx port.Tx, s port.SourceState) error {
	_, err := txFrom(tx).Exec(ctx, `
INSERT INTO system.service_source_states(service_id,source_type,status,reason,updated_at,last_change_at)
VALUES($1,$2,$3,$4,now(),now())
ON CONFLICT (service_id, source_type) DO UPDATE SET
  status = EXCLUDED.status,
  reason = EXCLUDED.reason,
  last_change_at = CASE WHEN system.service_source_states.status <> EXCLUDED.status THEN now() ELSE system.service_source_states.last_change_at END,
  updated_at = now()`,
		s.ServiceID, s.SourceType, s.Status, s.Reason)
	return Translate(err)
}