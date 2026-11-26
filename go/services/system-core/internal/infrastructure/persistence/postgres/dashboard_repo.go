package postgres

import (
	"context"
	"time"

	portdashboard "github.com/ecakeman/alert/services/system-core/internal/application/port/dashboard"
	portshared "github.com/ecakeman/alert/services/system-core/internal/application/port/shared"
)

type dashboardRepo struct{}

func NewDashboardRepo() portdashboard.DashboardRepo { return dashboardRepo{} }

// Summary 单事务内 3 组查询：13 项计数、最近 5 条故障、window 窗口小时趋势。
func (dashboardRepo) Summary(ctx context.Context, tx portshared.Tx, now time.Time, window time.Duration) (portdashboard.DashboardSummary, error) {
	s := portdashboard.DashboardSummary{
		GeneratedAt: now,
		Services:    map[string]int{},
		Targets:     map[string]int{},
		Episodes:    map[string]int{},
		Recent:      []portdashboard.RecentEpisode{},
		Trend:       []portdashboard.TrendPoint{},
	}

	var svcTotal, svcEnabled, svcDisabled, svcNormal, svcAbnormal, svcUnknown int
	var tgtTotal, tgtNormal, tgtAbnormal, tgtUnknown int
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
 (SELECT count(*) FROM system.service_monitor_targets WHERE deleted_at IS NULL AND status='UNKNOWN')`).
		Scan(&svcTotal, &svcEnabled, &svcDisabled, &svcNormal, &svcAbnormal, &svcUnknown,
			&tgtTotal, &tgtNormal, &tgtAbnormal, &tgtUnknown)
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
		var p portdashboard.TrendPoint
		if err := trendRows.Scan(&p.Bucket, &p.Success, &p.Failure); err != nil {
			return s, Translate(err)
		}
		p.Bucket = p.Bucket.UTC()
		s.Trend = append(s.Trend, p)
	}
	return s, Translate(trendRows.Err())
}

// EnrichRecent 用单体自有表补齐 service_name / 活动任务 / 诊断报告标记（≤5 条）。
func (dashboardRepo) EnrichRecent(ctx context.Context, tx portshared.Tx, items []portdashboard.RecentEpisode) ([]portdashboard.RecentEpisode, error) {
	for i := range items {
		err := txFrom(tx).QueryRow(ctx, `
SELECT s.name,
       COALESCE(at.status,''), COALESCE(at.task_type,''),
       EXISTS(SELECT 1 FROM system.episode_artifact_refs ar
              WHERE ar.episode_id = $1::uuid AND ar.artifact_type='DIAGNOSIS_REPORT')
FROM system.services s
LEFT JOIN LATERAL (
  SELECT t.status, t.task_type FROM system.agent_tasks t
  WHERE t.episode_id = $1::uuid AND t.status IN ('PENDING','DISPATCHED','RUNNING','RETRY_WAIT')
  ORDER BY t.created_at DESC LIMIT 1
) at ON true
WHERE s.service_id = $2::uuid`, items[i].EpisodeID, items[i].ServiceID).
			Scan(&items[i].ServiceName, &items[i].ActiveTaskStatus, &items[i].ActiveTaskType, &items[i].HasDiagnosisReport)
		if err != nil && !NoRows(err) {
			return nil, Translate(err)
		}
	}
	return items, nil
}
