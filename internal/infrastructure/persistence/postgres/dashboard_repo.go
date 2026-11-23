package postgres

import (
	"context"
	"time"

	"github.com/ecakeman/alert/internal/application/port"
)

type dashboardRepo struct{}

func NewDashboardRepo() port.DashboardRepo { return dashboardRepo{} }

func (dashboardRepo) Summary(ctx context.Context, tx port.Tx, now time.Time) (port.DashboardSummary, error) {
	s := port.DashboardSummary{
		GeneratedAt: now,
		Services:    map[string]int{},
		Targets:     map[string]int{},
		Episodes:    map[string]int{},
	}
	var svcTotal, svcEnabled, svcDisabled, svcNormal, svcAbnormal, svcUnknown int
	var tgtTotal, tgtNormal, tgtAbnormal, tgtUnknown int
	var epActive, epResolved, epArchived int
	err := txFrom(tx).QueryRow(ctx, `
SELECT
 (SELECT count(*) FROM system.services WHERE deleted_at IS NULL),
 (SELECT count(*) FROM system.services WHERE deleted_at IS NULL AND enabled),
 (SELECT count(*) FROM system.services WHERE deleted_at IS NULL AND NOT enabled),
 (SELECT count(*) FROM system.service_source_states WHERE source_type='MONITOR' AND status='NORMAL'),
 (SELECT count(*) FROM system.service_source_states WHERE source_type='MONITOR' AND status='ABNORMAL'),
 (SELECT count(*) FROM system.service_source_states WHERE source_type='MONITOR' AND status='UNKNOWN'),
 (SELECT count(*) FROM system.service_monitor_targets WHERE deleted_at IS NULL),
 (SELECT count(*) FROM system.service_monitor_targets WHERE deleted_at IS NULL AND status='NORMAL'),
 (SELECT count(*) FROM system.service_monitor_targets WHERE deleted_at IS NULL AND status='ABNORMAL'),
 (SELECT count(*) FROM system.service_monitor_targets WHERE deleted_at IS NULL AND status='UNKNOWN'),
 (SELECT count(*) FROM system.episodes WHERE status='ACTIVE'),
 (SELECT count(*) FROM system.episodes WHERE status='RESOLVED'),
 (SELECT count(*) FROM system.episodes WHERE status='ARCHIVED')`).
		Scan(&svcTotal, &svcEnabled, &svcDisabled, &svcNormal, &svcAbnormal, &svcUnknown,
			&tgtTotal, &tgtNormal, &tgtAbnormal, &tgtUnknown, &epActive, &epResolved, &epArchived)
	if err != nil {
		return s, Translate(err)
	}
	s.Services["total"] = svcTotal
	s.Services["enabled"] = svcEnabled
	s.Services["disabled"] = svcDisabled
	s.Services["normal"] = svcNormal
	s.Services["abnormal"] = svcAbnormal
	s.Services["unknown"] = svcUnknown
	s.Targets["total"] = tgtTotal
	s.Targets["normal"] = tgtNormal
	s.Targets["abnormal"] = tgtAbnormal
	s.Targets["unknown"] = tgtUnknown
	s.Episodes["active"] = epActive
	s.Episodes["resolved"] = epResolved
	s.Episodes["archived"] = epArchived

	rows, err := txFrom(tx).Query(ctx,
		`SELECT `+episodeColumns+` FROM system.episodes ORDER BY started_at DESC LIMIT 5`)
	if err != nil {
		return s, Translate(err)
	}
	defer rows.Close()
	for rows.Next() {
		e, err := scanEpisode(rows)
		if err != nil {
			return s, err
		}
		s.RecentEpisodes = append(s.RecentEpisodes, e)
	}
	return s, Translate(rows.Err())
}

func (d dashboardRepo) Counts(ctx context.Context, tx port.Tx) (map[string]int, error) {
	s, err := d.Summary(ctx, tx, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	out := map[string]int{}
	for k, v := range s.Services {
		out["services_"+k] = v
	}
	for k, v := range s.Targets {
		out["targets_"+k] = v
	}
	for k, v := range s.Episodes {
		out["episodes_"+k] = v
	}
	return out, nil
}

func (dashboardRepo) Trend(ctx context.Context, tx port.Tx, since time.Time) ([]port.TrendPoint, error) {
	rows, err := txFrom(tx).Query(ctx, `
SELECT date_trunc('hour', observed_at) AS bucket, count(*)
FROM system.service_monitor_points
WHERE observed_at >= $1
GROUP BY 1 ORDER BY 1`, utc(since))
	if err != nil {
		return nil, Translate(err)
	}
	defer rows.Close()
	var out []port.TrendPoint
	for rows.Next() {
		var p port.TrendPoint
		if err := rows.Scan(&p.Bucket, &p.Count); err != nil {
			return nil, Translate(err)
		}
		p.Bucket = p.Bucket.UTC()
		out = append(out, p)
	}
	return out, Translate(rows.Err())
}
