package postgres

import (
	"context"
	"time"

	"github.com/ecakeman/alert/internal/application/port"
)

type dashboardRepo struct{}

func NewDashboardRepo() port.DashboardRepo { return dashboardRepo{} }

// Summary 单事务内 3 组查询：13 项计数、最近 5 条故障、window 窗口小时趋势。
func (dashboardRepo) Summary(ctx context.Context, tx port.Tx, now time.Time, window time.Duration) (port.DashboardSummary, error) {
	s := port.DashboardSummary{
		GeneratedAt: now,
		Services:    map[string]int{},
		Targets:     map[string]int{},
		Episodes:    map[string]int{},
		Recent:      []port.RecentEpisode{},
		Trend:       []port.TrendPoint{},
	}

	var svcTotal, svcEnabled, svcDisabled, svcNormal, svcAbnormal, svcUnknown int
	var tgtTotal, tgtNormal, tgtAbnormal, tgtUnknown int
	var epActive, epResolved, epArchived int
	err := txFrom(tx).QueryRow(ctx, `
SELECT
 (SELECT count(*) FROM system.services WHERE deleted_at IS NULL),
 (SELECT count(*) FROM system.services WHERE deleted_at IS NULL AND enabled),
 (SELECT count(*) FROM system.services WHERE deleted_at IS NULL AND NOT enabled),
 (SELECT count(*) FROM system.service_source_states ss
    JOIN system.services s2 ON s2.service_id = ss.service_id
    WHERE ss.source_type='MONITOR' AND s2.deleted_at IS NULL AND ss.status='NORMAL'),
 (SELECT count(*) FROM system.service_source_states ss
    JOIN system.services s2 ON s2.service_id = ss.service_id
    WHERE ss.source_type='MONITOR' AND s2.deleted_at IS NULL AND ss.status='ABNORMAL'),
 (SELECT count(*) FROM system.service_source_states ss
    JOIN system.services s2 ON s2.service_id = ss.service_id
    WHERE ss.source_type='MONITOR' AND s2.deleted_at IS NULL AND ss.status='UNKNOWN'),
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

	recentRows, err := txFrom(tx).Query(ctx, `
SELECT e.episode_id, e.service_id, s.name, e.status, e.resolution_pending, e.started_by,
       e.started_at, e.resolved_at, e.archived_at, e.input_version, e.processed_input_version, e.updated_at,
       COALESCE(at.status,''), COALESCE(at.task_type,''),
       EXISTS(SELECT 1 FROM system.episode_artifact_refs ar
              WHERE ar.episode_id=e.episode_id AND ar.artifact_type='DIAGNOSIS_REPORT')
FROM system.episodes e
JOIN system.services s ON s.service_id = e.service_id
LEFT JOIN LATERAL (
  SELECT t.status, t.task_type FROM system.agent_tasks t
  WHERE t.episode_id = e.episode_id AND t.status IN ('PENDING','DISPATCHED','RUNNING','RETRY_WAIT')
  ORDER BY t.created_at DESC LIMIT 1
) at ON true
ORDER BY e.started_at DESC
LIMIT 5`)
	if err != nil {
		return s, Translate(err)
	}
	defer recentRows.Close()
	for recentRows.Next() {
		var r port.RecentEpisode
		if err := recentRows.Scan(&r.EpisodeID, &r.ServiceID, &r.ServiceName, &r.Status,
			&r.ResolutionPending, &r.StartedBy, &r.StartedAt, &r.ResolvedAt, &r.ArchivedAt,
			&r.InputVersion, &r.ProcessedInputVersion, &r.UpdatedAt,
			&r.ActiveTaskStatus, &r.ActiveTaskType, &r.HasDiagnosisReport); err != nil {
			return s, Translate(err)
		}
		s.Recent = append(s.Recent, r)
	}
	if err := recentRows.Err(); err != nil {
		return s, Translate(err)
	}

	trendRows, err := txFrom(tx).Query(ctx, `
SELECT date_trunc('hour', observed_at) AS bucket,
       count(*) FILTER (WHERE check_status='SUCCESS') AS success,
       count(*) FILTER (WHERE check_status <> 'SUCCESS') AS failure
FROM system.service_monitor_points
WHERE observed_at >= now() - make_interval(secs => $1::int)
GROUP BY 1 ORDER BY 1`, int(window.Seconds()))
	if err != nil {
		return s, Translate(err)
	}
	defer trendRows.Close()
	for trendRows.Next() {
		var p port.TrendPoint
		if err := trendRows.Scan(&p.Bucket, &p.Success, &p.Failure); err != nil {
			return s, Translate(err)
		}
		p.Bucket = p.Bucket.UTC()
		s.Trend = append(s.Trend, p)
	}
	return s, Translate(trendRows.Err())
}
