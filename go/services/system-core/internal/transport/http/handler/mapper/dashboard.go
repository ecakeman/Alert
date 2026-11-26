package mapper

import (
	"time"

	portdashboard "github.com/ecakeman/alert/services/system-core/internal/application/port/dashboard"
	"github.com/ecakeman/alert/services/system-core/internal/transport/http/dto"
)

// ToRecentEpisodeSummary 委托 episode 的唯一公共字段填充实现，
// 差异字段 HasDiagnosisReport 恒写（dashboard 读模型语义）。
func ToRecentEpisodeSummary(r portdashboard.RecentEpisode) dto.EpisodeSummary {
	out := fillEpisodeSummary(episodeSummaryCore{
		EpisodeID: r.EpisodeID, ServiceID: r.ServiceID, ServiceName: r.ServiceName,
		Status: r.Status, ResolutionPending: r.ResolutionPending, StartedBy: r.StartedBy,
		StartedAt: r.StartedAt, ResolvedAt: r.ResolvedAt, ArchivedAt: r.ArchivedAt,
		InputVersion: r.InputVersion, ProcessedInputVersion: r.ProcessedInputVersion,
		ActiveTaskStatus: r.ActiveTaskStatus, ActiveTaskType: r.ActiveTaskType,
		UpdatedAt: r.UpdatedAt,
	})
	has := r.HasDiagnosisReport
	out.HasDiagnosisReport = &has
	return out
}

func ToTrendPoint(p portdashboard.TrendPoint) dto.MonitorTrendPoint {
	return dto.MonitorTrendPoint{
		BucketStart: p.Bucket.UTC().Format(time.RFC3339),
		Success:     p.Success,
		Failure:     p.Failure,
	}
}
