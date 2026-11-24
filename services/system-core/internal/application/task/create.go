package task

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ecakeman/alert/services/system-core/internal/application/apperr"
	portshared "github.com/ecakeman/alert/services/system-core/internal/application/port/shared"
	porttask "github.com/ecakeman/alert/services/system-core/internal/application/port/task"
	"github.com/ecakeman/alert/services/system-core/internal/contracts"
	"github.com/ecakeman/alert/services/system-core/internal/domain"
)

const (
	TaskTypeDiagnosis = "DIAGNOSIS"
	TaskTypeArchive   = "ARCHIVE"
)

// TaskSource 是任务创建触发来源。
type TaskSource string

const (
	SourceEpisodeActive   TaskSource = "EPISODE_ACTIVE"
	SourceInputAppended   TaskSource = "INPUT_APPENDED"
	SourceEpisodeResolved TaskSource = "EPISODE_RESOLVED"
	SourceManualRetry     TaskSource = "MANUAL_RETRY"
)

// CreateInput 是任务创建输入。
type CreateInput struct {
	EpisodeID string
	ServiceID string
	TaskType  string
	Source    TaskSource
}

func validSource(taskType string, source TaskSource) bool {
	switch taskType {
	case TaskTypeDiagnosis:
		return source == SourceEpisodeActive || source == SourceInputAppended
	case TaskTypeArchive:
		return source == SourceEpisodeResolved || source == SourceManualRetry
	default:
		return false
	}
}

// Create 在单事务内冻结输入、写任务（PENDING）与派发事件；bool=是否复用既有活动任务。
func (s *Service) Create(ctx context.Context, in CreateInput) (TaskView, bool, error) {
	if !validSource(in.TaskType, in.Source) {
		return TaskView{}, false, apperr.Validation("task type and source mismatch")
	}
	var view TaskView
	var reused bool
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		episode, err := s.deps.Episodes.Get(ctx, tx, in.EpisodeID)
		if err != nil {
			return err
		}
		active, err := s.deps.Tasks.ListActiveByEpisode(ctx, tx, in.EpisodeID)
		if err != nil {
			return err
		}
		for _, existing := range active {
			if existing.TaskType == in.TaskType {
				view = toTaskView(existing)
				reused = true
				return nil
			}
		}
		if len(active) > 0 {
			return apperr.New(domain.CodeStateConflict, "another task type is active for this episode")
		}

		latest, err := s.deps.InputVersions.GetLatest(ctx, tx, in.EpisodeID)
		if err != nil {
			return err
		}
		from := episode.ProcessedInputVersion + 1
		to := episode.InputVersion
		if latest.InputVersion > to {
			to = latest.InputVersion
		}
		if in.TaskType == TaskTypeArchive {
			from = to
		}
		if in.TaskType == TaskTypeDiagnosis && from > to {
			return nil
		}

		now := s.deps.Clock.Now()
		task := porttask.AgentTask{
			TaskID:            s.deps.IDs.NewID(),
			EpisodeID:         episode.EpisodeID,
			ServiceID:         episode.ServiceID,
			TaskType:          in.TaskType,
			Status:            string(domain.TaskPending),
			Generation:        1,
			MaxAttempts:       s.deps.MaxAttempts,
			InputVersionFrom:  from,
			InputVersionTo:    to,
			MonitorWatermark:  episode.MonitorWatermark,
			AppEventWatermark: episode.AppEventWatermark,
			DeadlineAt:        now.Add(s.deps.Deadline),
			CreatedAt:         now,
			UpdatedAt:         now,
		}
		if err := s.deps.Tasks.Create(ctx, tx, task); err != nil {
			return err
		}

		payload, err := json.Marshal(map[string]any{
			"task_id":    task.TaskID,
			"generation": task.Generation,
		})
		if err != nil {
			return err
		}
		dedupe := fmt.Sprintf("task.dispatch:%s:%d", task.TaskID, task.Generation)
		event := contracts.Event{
			EventID:       s.deps.IDs.NewID(),
			EventType:     contracts.EventTaskDispatchRequested,
			SchemaVersion: 1,
			AggregateType: "task",
			AggregateID:   task.TaskID,
			OccurredAt:    now,
			Payload:       payload,
		}
		if err := s.deps.Outbox.Append(ctx, tx, event, &dedupe); err != nil {
			return err
		}
		view = toTaskView(task)
		return nil
	})
	return view, reused, err
}

func toTaskView(t porttask.AgentTask) TaskView {
	return TaskView{
		TaskID:           t.TaskID,
		EpisodeID:        t.EpisodeID,
		ServiceID:        t.ServiceID,
		TaskType:         t.TaskType,
		Status:           t.Status,
		Generation:       t.Generation,
		Attempt:          t.Attempt,
		MaxAttempts:      t.MaxAttempts,
		LeaseUntil:       t.LeaseUntil,
		NextRetryAt:      t.NextRetryAt,
		InputVersionFrom: t.InputVersionFrom,
		InputVersionTo:   t.InputVersionTo,
		MonitorWatermark: t.MonitorWatermark,
		AppWatermark:     t.AppEventWatermark,
		DeadlineAt:       t.DeadlineAt,
		ErrorCode:        t.ErrorCode,
		ErrorMessage:     t.ErrorMessage,
		CreatedAt:        t.CreatedAt,
		UpdatedAt:        t.UpdatedAt,
		StartedAt:        t.StartedAt,
		FinishedAt:       t.FinishedAt,
	}
}
