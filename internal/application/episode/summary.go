package episode

import (
	"context"

	"github.com/ecakeman/alert/internal/application/apperr"
	"github.com/ecakeman/alert/internal/application/port"
)

func (s *Service) enrichEpisode(ctx context.Context, tx port.Tx, ep port.Episode) (EpisodeSummaryData, error) {
	svc, err := s.deps.Services.Get(ctx, tx, ep.ServiceID)
	if err != nil {
		return EpisodeSummaryData{}, err
	}
	d := EpisodeSummaryData{Episode: ep, ServiceName: svc.Name}

	tasks, err := s.deps.Tasks.ListActiveByEpisode(ctx, tx, ep.EpisodeID)
	if err != nil {
		return EpisodeSummaryData{}, err
	}
	if len(tasks) > 0 {
		d.ActiveTaskStatus = tasks[0].Status
		d.ActiveTaskType = tasks[0].TaskType
	}

	if _, err := s.deps.Artifacts.GetLatest(ctx, tx, ep.EpisodeID, "DIAGNOSIS_REPORT"); err == nil {
		d.HasDiagnosisReport = true
	} else if !isAppCode(err, apperr.CodeNotFound) {
		return EpisodeSummaryData{}, err
	}
	return d, nil
}

// ListSummaries 返回故障列表聚合（含服务名、活动任务、诊断报告标记）。
func (s *Service) ListSummaries(ctx context.Context, f port.EpisodeFilter) ([]EpisodeSummaryData, error) {
	var out []EpisodeSummaryData
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx port.Tx) error {
		eps, err := s.deps.Episodes.List(ctx, tx, f)
		if err != nil {
			return err
		}
		for _, ep := range eps {
			d, err := s.enrichEpisode(ctx, tx, ep)
			if err != nil {
				return err
			}
			out = append(out, d)
		}
		return nil
	})
	return out, err
}
