package task

import (
	"context"
	"fmt"

	"github.com/ecakeman/alert/services/system-core/internal/contracts"
	"github.com/ecakeman/alert/services/system-core/internal/domain"
)

const schedulerBatch = 32

// EnsureDiagnosisTasks 对账兜底：从 episode 域查询待派发快照，补建 DIAGNOSIS。
// 主路径是 HandleDispatchableEvent；本函数只处理事件缺失的情况。
func (s *Service) EnsureDiagnosisTasks(ctx context.Context, limit int) (int, error) {
	snapshots, err := s.pendingDispatch(ctx, limit)
	if err != nil {
		return 0, err
	}
	created := 0
	for _, snapshot := range snapshots {
		if snapshot.EpisodeStatus != string(domain.IncidentActive) {
			continue
		}
		view, reused, err := s.Create(ctx, CreateInput{
			Snapshot: snapshot,
			TaskType: TaskTypeDiagnosis,
			Source:   SourceEpisodeActive,
		})
		if err != nil {
			continue
		}
		if !reused && view.TaskID != "" {
			created++
		}
	}
	return created, nil
}

// EnsureArchiveTasks 对账兜底：RESOLVED 且产物齐全时补建 ARCHIVE。
func (s *Service) EnsureArchiveTasks(ctx context.Context, limit int) (int, error) {
	snapshots, err := s.pendingDispatch(ctx, limit)
	if err != nil {
		return 0, err
	}
	created := 0
	for _, snapshot := range snapshots {
		if snapshot.EpisodeStatus != string(domain.IncidentResolved) {
			continue
		}
		view, reused, err := s.Create(ctx, CreateInput{
			Snapshot: snapshot,
			TaskType: TaskTypeArchive,
			Source:   SourceEpisodeResolved,
		})
		if err != nil {
			continue
		}
		if !reused && view.TaskID != "" {
			created++
		}
	}
	return created, nil
}

func (s *Service) pendingDispatch(ctx context.Context, limit int) ([]contracts.DispatchSnapshot, error) {
	if limit <= 0 || limit > schedulerBatch {
		limit = schedulerBatch
	}
	if s.deps.Dispatch == nil {
		return nil, fmt.Errorf("task dispatch source is not configured")
	}
	return s.deps.Dispatch.ListPendingDispatch(ctx, limit)
}
