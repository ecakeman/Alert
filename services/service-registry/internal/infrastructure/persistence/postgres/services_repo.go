package postgres

import (
	"context"
	"time"

	portService "github.com/ecakeman/alert/services/service-registry/internal/application/port/service"
	portshared "github.com/ecakeman/alert/services/service-registry/internal/application/port/shared"
	"github.com/ecakeman/alert/services/service-registry/internal/infrastructure/persistence/postgres/model"
)

type serviceRepo struct{}

func NewServiceRepo() portService.ServiceRepo { return serviceRepo{} }

// Create：created_at/updated_at 由 DB DEFAULT now() 接管（应用类型已去存储时间字段）。
func (serviceRepo) Create(ctx context.Context, tx portshared.Tx, s portService.Service) error {
	row := model.ServiceRowFromApp(s)
	_, err := txFrom(tx).Exec(ctx, `
INSERT INTO registry.services(service_id,service_key,name,description,enabled,monitor_aggregation_mode,authority_producer_id)
VALUES($1,$2,$3,$4,$5,$6,$7)`,
		row.ServiceID, row.ServiceKey, row.Name, row.Description, row.Enabled,
		row.MonitorAggregationMode, row.AuthorityProducerID)
	return Translate(err)
}

func (serviceRepo) Get(ctx context.Context, tx portshared.Tx, serviceID string) (portService.Service, error) {
	row, err := model.ServiceRow{}.Scan(txFrom(tx).QueryRow(ctx,
		`SELECT `+model.ServiceColumns+` FROM registry.services WHERE service_id=$1 AND deleted_at IS NULL`, serviceID))
	if err != nil {
		return portService.Service{}, Translate(err)
	}
	return row.ToApp(), nil
}

func (serviceRepo) GetByKey(ctx context.Context, tx portshared.Tx, serviceKey string) (portService.Service, error) {
	row, err := model.ServiceRow{}.Scan(txFrom(tx).QueryRow(ctx,
		`SELECT `+model.ServiceColumns+` FROM registry.services WHERE service_key=$1`, serviceKey))
	if err != nil {
		return portService.Service{}, Translate(err)
	}
	return row.ToApp(), nil
}

// serviceFilterWhere 生成 query/status 过滤 WHERE 子句与参数（List/Count 共用）。
// status=UNKNOWN 同时命中“MONITOR 来源状态为 UNKNOWN”与“无 MONITOR 来源行”的服务。
func serviceFilterWhere(f portService.ServiceFilter) (string, []any) {
	where := `deleted_at IS NULL
  AND ($1 = '' OR name ILIKE '%'||$1||'%' OR service_key ILIKE '%'||$1||'%')
  AND (
    $2 = ''
    OR EXISTS (SELECT 1 FROM registry.service_source_states st
               WHERE st.service_id = services.service_id AND st.source_type='MONITOR' AND st.status=$2)
    OR ($2 = 'UNKNOWN' AND NOT EXISTS (SELECT 1 FROM registry.service_source_states st2
               WHERE st2.service_id = services.service_id AND st2.source_type='MONITOR'))
  )`
	return where, []any{f.Query, f.Status}
}

func (serviceRepo) Count(ctx context.Context, tx portshared.Tx, f portService.ServiceFilter) (int, error) {
	where, args := serviceFilterWhere(f)
	var total int
	err := txFrom(tx).QueryRow(ctx,
		`SELECT count(*) FROM registry.services WHERE `+where, args...).Scan(&total)
	if err != nil {
		return 0, Translate(err)
	}
	return total, nil
}

func (serviceRepo) List(ctx context.Context, tx portshared.Tx, f portService.ServiceFilter) ([]portService.Service, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	where, args := serviceFilterWhere(f)
	rows, err := txFrom(tx).Query(ctx, `
SELECT `+model.ServiceColumns+` FROM registry.services
WHERE `+where+`
ORDER BY created_at DESC
LIMIT $3 OFFSET $4`, append(args, limit, f.Offset)...)
	if err != nil {
		return nil, Translate(err)
	}
	defer rows.Close()
	var out []portService.Service
	for rows.Next() {
		row, err := model.ServiceRow{}.Scan(rows)
		if err != nil {
			return nil, Translate(err)
		}
		out = append(out, row.ToApp())
	}
	return out, Translate(rows.Err())
}

func (serviceRepo) Patch(ctx context.Context, tx portshared.Tx, serviceID string, p portService.ServicePatch) error {
	_, err := txFrom(tx).Exec(ctx, `
UPDATE registry.services SET
  name = COALESCE($2, name),
  description = COALESCE($3, description),
  enabled = COALESCE($4, enabled),
  monitor_aggregation_mode = COALESCE($5, monitor_aggregation_mode),
  updated_at = now()
WHERE service_id=$1 AND deleted_at IS NULL`,
		serviceID, p.Name, p.Description, p.Enabled, p.MonitorAggregationMode)
	return Translate(err)
}

func (serviceRepo) SoftDelete(ctx context.Context, tx portshared.Tx, serviceID string, at time.Time) error {
	_, err := txFrom(tx).Exec(ctx, `
UPDATE registry.services SET deleted_at=$2, enabled=false, updated_at=now()
WHERE service_id=$1 AND deleted_at IS NULL`, serviceID, utc(at))
	return Translate(err)
}

// ListByIDs 批量取服务（ids 为空时返回空列表），用于 BFF/域内批量补齐。
func (serviceRepo) ListByIDs(ctx context.Context, tx portshared.Tx, ids []string) ([]portService.Service, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := txFrom(tx).Query(ctx, `
SELECT `+model.ServiceColumns+` FROM registry.services
WHERE service_id = ANY($1::uuid[]) AND deleted_at IS NULL
ORDER BY created_at`, ids)
	if err != nil {
		return nil, Translate(err)
	}
	defer rows.Close()
	var out []portService.Service
	for rows.Next() {
		row, err := model.ServiceRow{}.Scan(rows)
		if err != nil {
			return nil, Translate(err)
		}
		out = append(out, row.ToApp())
	}
	return out, Translate(rows.Err())
}
