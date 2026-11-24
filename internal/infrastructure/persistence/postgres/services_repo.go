package postgres

import (
	"context"
	"time"

	"github.com/ecakeman/alert/internal/application/port"
	"github.com/ecakeman/alert/internal/infrastructure/persistence/postgres/model"
)

type serviceRepo struct{}

func NewServiceRepo() port.ServiceRepo { return serviceRepo{} }

// Create：created_at/updated_at 由 DB DEFAULT now() 接管（应用类型已去存储时间字段）。
func (serviceRepo) Create(ctx context.Context, tx port.Tx, s port.Service) error {
	row := model.ServiceRowFromApp(s)
	_, err := txFrom(tx).Exec(ctx, `
INSERT INTO system.services(service_id,service_key,name,description,enabled,monitor_aggregation_mode,authority_producer_id)
VALUES($1,$2,$3,$4,$5,$6,$7)`,
		row.ServiceID, row.ServiceKey, row.Name, row.Description, row.Enabled,
		row.MonitorAggregationMode, row.AuthorityProducerID)
	return Translate(err)
}

func (serviceRepo) Get(ctx context.Context, tx port.Tx, serviceID string) (port.Service, error) {
	row, err := model.ServiceRow{}.Scan(txFrom(tx).QueryRow(ctx,
		`SELECT `+model.ServiceColumns+` FROM system.services WHERE service_id=$1`, serviceID))
	if err != nil {
		return port.Service{}, Translate(err)
	}
	return row.ToApp(), nil
}

func (serviceRepo) GetByKey(ctx context.Context, tx port.Tx, serviceKey string) (port.Service, error) {
	row, err := model.ServiceRow{}.Scan(txFrom(tx).QueryRow(ctx,
		`SELECT `+model.ServiceColumns+` FROM system.services WHERE service_key=$1`, serviceKey))
	if err != nil {
		return port.Service{}, Translate(err)
	}
	return row.ToApp(), nil
}

// serviceFilterWhere 生成 query/status 过滤 WHERE 子句与参数（List/Count 共用）。
// status=UNKNOWN 同时命中“MONITOR 来源状态为 UNKNOWN”与“无 MONITOR 来源行”的服务。
func serviceFilterWhere(f port.ServiceFilter) (string, []any) {
	where := `deleted_at IS NULL
  AND ($1 = '' OR name ILIKE '%'||$1||'%' OR service_key ILIKE '%'||$1||'%')
  AND (
    $2 = ''
    OR EXISTS (SELECT 1 FROM system.service_source_states st
               WHERE st.service_id = services.service_id AND st.source_type='MONITOR' AND st.status=$2)
    OR ($2 = 'UNKNOWN' AND NOT EXISTS (SELECT 1 FROM system.service_source_states st2
               WHERE st2.service_id = services.service_id AND st2.source_type='MONITOR'))
  )`
	return where, []any{f.Query, f.Status}
}

func (serviceRepo) Count(ctx context.Context, tx port.Tx, f port.ServiceFilter) (int, error) {
	where, args := serviceFilterWhere(f)
	var total int
	err := txFrom(tx).QueryRow(ctx,
		`SELECT count(*) FROM system.services WHERE `+where, args...).Scan(&total)
	if err != nil {
		return 0, Translate(err)
	}
	return total, nil
}

func (serviceRepo) List(ctx context.Context, tx port.Tx, f port.ServiceFilter) ([]port.Service, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	where, args := serviceFilterWhere(f)
	rows, err := txFrom(tx).Query(ctx, `
SELECT `+model.ServiceColumns+` FROM system.services
WHERE `+where+`
ORDER BY created_at DESC
LIMIT $3 OFFSET $4`, append(args, limit, f.Offset)...)
	if err != nil {
		return nil, Translate(err)
	}
	defer rows.Close()
	var out []port.Service
	for rows.Next() {
		row, err := model.ServiceRow{}.Scan(rows)
		if err != nil {
			return nil, Translate(err)
		}
		out = append(out, row.ToApp())
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
