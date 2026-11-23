package postgres

import (
	"context"
	"time"

	"github.com/ecakeman/alert/internal/application/port"
)

type serviceRepo struct{}

func NewServiceRepo() port.ServiceRepo { return serviceRepo{} }

const serviceColumns = `service_id,service_key,name,description,enabled,monitor_aggregation_mode,authority_producer_id,deleted_at,created_at,updated_at`

func scanService(row scannable) (port.Service, error) {
	var s port.Service
	if err := row.Scan(&s.ServiceID, &s.ServiceKey, &s.Name, &s.Description, &s.Enabled,
		&s.MonitorAggregationMode, &s.AuthorityProducerID, &s.DeletedAt, &s.CreatedAt, &s.UpdatedAt); err != nil {
		return port.Service{}, Translate(err)
	}
	return s, nil
}

func (serviceRepo) Create(ctx context.Context, tx port.Tx, s port.Service) error {
	_, err := txFrom(tx).Exec(ctx, `
INSERT INTO system.services(service_id,service_key,name,description,enabled,monitor_aggregation_mode,authority_producer_id,created_at,updated_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		s.ServiceID, s.ServiceKey, s.Name, s.Description, s.Enabled, s.MonitorAggregationMode,
		s.AuthorityProducerID, utc(s.CreatedAt), utc(s.UpdatedAt))
	return Translate(err)
}

func (serviceRepo) Get(ctx context.Context, tx port.Tx, serviceID string) (port.Service, error) {
	return scanService(txFrom(tx).QueryRow(ctx, `SELECT `+serviceColumns+` FROM system.services WHERE service_id=$1`, serviceID))
}

func (serviceRepo) GetByKey(ctx context.Context, tx port.Tx, serviceKey string) (port.Service, error) {
	return scanService(txFrom(tx).QueryRow(ctx, `SELECT `+serviceColumns+` FROM system.services WHERE service_key=$1`, serviceKey))
}

func (serviceRepo) List(ctx context.Context, tx port.Tx, f port.ServiceFilter) ([]port.Service, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	rows, err := txFrom(tx).Query(ctx, `
SELECT `+serviceColumns+` FROM system.services
WHERE deleted_at IS NULL
  AND ($1 = '' OR name ILIKE '%'||$1||'%' OR service_key ILIKE '%'||$1||'%')
ORDER BY created_at DESC
LIMIT $2 OFFSET $3`, f.Query, limit, f.Offset)
	if err != nil {
		return nil, Translate(err)
	}
	defer rows.Close()
	var out []port.Service
	for rows.Next() {
		s, err := scanService(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, Translate(rows.Err())
}

func (serviceRepo) Patch(ctx context.Context, tx port.Tx, serviceID string, p port.ServicePatch) error {
	_, err := txFrom(tx).Exec(ctx, `
UPDATE system.services SET
  name = COALESCE($2, name),
  description = COALESCE($3, description),
  enabled = COALESCE($4, enabled),
  monitor_aggregation_mode = COALESCE($5, monitor_aggregation_mode),
  updated_at = now()
WHERE service_id=$1 AND deleted_at IS NULL`,
		serviceID, p.Name, p.Description, p.Enabled, p.MonitorAggregationMode)
	return Translate(err)
}

func (serviceRepo) SoftDelete(ctx context.Context, tx port.Tx, serviceID string, at time.Time) error {
	_, err := txFrom(tx).Exec(ctx, `
UPDATE system.services SET deleted_at=$2, enabled=false, updated_at=now()
WHERE service_id=$1 AND deleted_at IS NULL`, serviceID, utc(at))
	return Translate(err)
}