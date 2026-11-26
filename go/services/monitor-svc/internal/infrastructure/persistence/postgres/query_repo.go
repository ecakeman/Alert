package postgres

import (
	"context"
	"time"

	portshared "github.com/ecakeman/alert/services/monitor-svc/internal/application/port/shared"
)

// TrendPoint 是一个整小时趋势桶。
type TrendPoint struct {
	Bucket  time.Time
	Success int
	Failure int
}

// MonitorStats 是 monitor 视角计数。
type MonitorStats struct {
	Services map[string]int
	Targets  map[string]int
}

type queryRepo struct{}

func NewQueryRepo() queryRepo { return queryRepo{} }

func (queryRepo) Trend(ctx context.Context, tx portshared.Tx, now time.Time, window time.Duration, buckets int) ([]TrendPoint, error) {
	rows, err := txFrom(tx).Query(ctx, `
SELECT date_trunc('hour', observed_at) AS bucket,
       count(*) FILTER (WHERE check_status='SUCCESS') AS success,
       count(*) FILTER (WHERE check_status <> 'SUCCESS') AS failure
FROM monitor.service_monitor_points
WHERE observed_at >= $1::timestamptz - make_interval(secs => $2::int)
GROUP BY 1 ORDER BY 1`, utc(now), int(window.Seconds()))
	if err != nil {
		return nil, Translate(err)
	}
	defer rows.Close()
	byBucket := map[time.Time]TrendPoint{}
	for rows.Next() {
		var p TrendPoint
		if err := rows.Scan(&p.Bucket, &p.Success, &p.Failure); err != nil {
			return nil, Translate(err)
		}
		byBucket[p.Bucket.UTC()] = p
	}
	if err := rows.Err(); err != nil {
		return nil, Translate(err)
	}
	if buckets <= 0 {
		buckets = int(window / time.Hour)
	}
	// 以 now 所在整点为右边界，向前补齐 buckets 个桶（含零值），键集合固定。
	end := now.UTC().Truncate(time.Hour)
	out := make([]TrendPoint, 0, buckets)
	for i := buckets - 1; i >= 0; i-- {
		b := end.Add(-time.Duration(i) * time.Hour)
		p := byBucket[b]
		p.Bucket = b
		out = append(out, p)
	}
	return out, nil
}

func (queryRepo) Stats(ctx context.Context, tx portshared.Tx) (MonitorStats, error) {
	s := MonitorStats{Services: map[string]int{}, Targets: map[string]int{}}
	var tgtTotal int
	if err := txFrom(tx).QueryRow(ctx, `
SELECT count(*) FROM monitor.monitor_target_projection WHERE deleted_at IS NULL`).Scan(&tgtTotal); err != nil {
		return s, Translate(err)
	}
	s.Targets["total"] = tgtTotal
	s.Targets["normal"], s.Targets["abnormal"], s.Targets["unknown"] = 0, 0, 0

	rows, err := txFrom(tx).Query(ctx, `
SELECT status, count(*) FROM monitor.monitor_target_state GROUP BY 1`)
	if err != nil {
		return s, Translate(err)
	}
	for rows.Next() {
		var status string
		var n int
		if err := rows.Scan(&status, &n); err != nil {
			rows.Close()
			return s, Translate(err)
		}
		switch status {
		case "NORMAL":
			s.Targets["normal"] += n
		case "ABNORMAL":
			s.Targets["abnormal"] += n
		default:
			s.Targets["unknown"] += n
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return s, Translate(err)
	}

	s.Services["total"], s.Services["normal"], s.Services["abnormal"], s.Services["unknown"] = 0, 0, 0, 0
	srcRows, err := txFrom(tx).Query(ctx, `
SELECT status, count(*) FROM monitor.monitor_source_state GROUP BY 1`)
	if err != nil {
		return s, Translate(err)
	}
	defer srcRows.Close()
	for srcRows.Next() {
		var status string
		var n int
		if err := srcRows.Scan(&status, &n); err != nil {
			return s, Translate(err)
		}
		s.Services["total"] += n
		switch status {
		case "NORMAL":
			s.Services["normal"] += n
		case "ABNORMAL":
			s.Services["abnormal"] += n
		default:
			s.Services["unknown"] += n
		}
	}
	return s, Translate(srcRows.Err())
}