package postgres

import (
	"context"

	portshared "github.com/ecakeman/alert/services/service-registry/internal/application/port/shared"
)

// StatusCounts 返回未删除目标按状态分组计数（dashboard 计数用）。
func (targetRepo) StatusCounts(ctx context.Context, tx portshared.Tx) (map[string]int, error) {
	rows, err := txFrom(tx).Query(ctx, `
SELECT status, count(*) FROM registry.service_monitor_targets
WHERE deleted_at IS NULL
GROUP BY status`)
	if err != nil {
		return nil, Translate(err)
	}
	defer rows.Close()

	out := map[string]int{"NORMAL": 0, "ABNORMAL": 0, "UNKNOWN": 0}
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, Translate(err)
		}
		out[status] = count
	}
	return out, Translate(rows.Err())
}
