package task

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ecakeman/alert/services/task-orchestrator/internal/application/apperr"
	portoutbox "github.com/ecakeman/alert/services/task-orchestrator/internal/application/port/outbox"
	portshared "github.com/ecakeman/alert/services/task-orchestrator/internal/application/port/shared"
	porttask "github.com/ecakeman/alert/services/task-orchestrator/internal/application/port/task"
	"github.com/ecakeman/alert/services/contracts"
	"github.com/ecakeman/alert/services/task-orchestrator/internal/domain"
)

const (
	TaskTypeDiagnosis = "DIAGNOSIS"
	TaskTypeArchive   = "ARCHIVE"
)

// dispatchSchedulerConsumer 是 episode 派发快照的幂等消费者名（design 2.5.2）。
const dispatchSchedulerConsumer = "task.episode-events.v1"

// TaskSource 是任务创建触发来源。
type TaskSource string

const (
	SourceEpisodeActive   TaskSource = "EPISODE_ACTIVE"
	SourceInputAppended   TaskSource = "INPUT_APPENDED"
	SourceEpisodeResolved TaskSource = "EPISODE_RESOLVED"
	SourceManualRetry     TaskSource = "MANUAL_RETRY"
)

// CreateInput 是任务创建输入：快照由 episode 域提供，task 侧零回查。
type CreateInput struct {
	Snapshot  contracts.DispatchSnapshot
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

// Create 在单事务内消费快照建任务（PENDING）并写派发事件；bool=是否复用既有活动任务。
// 主路径不读取 episodes / input_versions；重复快照由幂等键拦截。
func (s *Service) Create(ctx context.Context, in CreateInput) (TaskView, bool, error) {
	if err := validateSnapshot(in.Snapshot); err != nil {
		return TaskView{}, false, apperr.Validation(err.Error())
	}
	if !validSource(in.TaskType, in.Source) {
		return TaskView{}, false, apperr.Validation("task type and source mismatch")
	}
	snap := in.Snapshot
	var view TaskView
	var reused bool

	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		taskID := s.deps.IDs.NewID()
		generation := int64(1)
		key := fmt.Sprintf("episode:%s:%d:%s", snap.EpisodeID, snap.InputVersionTo, snap.Reason)
		reserved, err := s.deps.Idem.Reserve(ctx, tx, portoutbox.ConsumerIdempotency{
			Consumer:       dispatchSchedulerConsumer,
			IdempotencyKey: key,
			TaskID:         &taskID,
			Generation:     &generation,
		})
		if err != nil {
			return err
		}
		if !reserved {
			return nil
		}

		active, err := s.deps.Tasks.ListActiveByEpisode(ctx, tx, snap.EpisodeID)
		if err != nil {
			return err
		}
		for _, existing := range active {
			if existing.TaskType == in.TaskType {
				view = toTaskView(existing)
				reused = true
				return s.markDispatched(ctx, tx, key, "REUSED")
			}
		}
		if len(active) > 0 {
			return apperr.New(domain.CodeStateConflict, "another task type is active for this episode")
		}

		from := snap.InputVersionFrom
		to := snap.InputVersionTo
		if in.TaskType == TaskTypeArchive {
			if !snap.ArtifactReadiness.Investigation || !snap.ArtifactReadiness.DiagnosisReport || !snap.ArtifactReadiness.HashesMatch {
				return s.markDispatched(ctx, tx, key, "SKIPPED_NOT_READY")
			}
			from = to
		}
		if in.TaskType == TaskTypeDiagnosis && from > to {
			return s.markDispatched(ctx, tx, key, "SKIPPED_NO_INPUT")
		}

		now := s.deps.Clock.Now()
		task := porttask.AgentTask{
			TaskID:            taskID,
			EpisodeID:         snap.EpisodeID,
			ServiceID:         snap.ServiceID,
			TaskType:          in.TaskType,
			Status:            string(domain.TaskPending),
			Generation:        generation,
			MaxAttempts:       s.deps.MaxAttempts,
			InputVersionFrom:  from,
			InputVersionTo:    to,
			MonitorWatermark:  snap.MonitorWatermark,
			AppEventWatermark: snap.AppEventWatermark,
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

func (s *Service) markDispatched(ctx context.Context, tx portshared.Tx, key, resultCode string) error {
	return s.deps.Idem.MarkDone(ctx, tx, dispatchSchedulerConsumer, key, resultCode, s.deps.Clock.Now())
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

// validateSnapshot 校验派发快照的身份字段与输入区间（共享契约不承载单侧校验方法）。
func validateSnapshot(s contracts.DispatchSnapshot) error {
	if s.EpisodeID == "" || s.ServiceID == "" || s.EpisodeStatus == "" || s.Reason == "" {
		return fmt.Errorf("dispatch snapshot missing identity fields")
	}
	if s.InputVersionFrom < 1 || s.InputVersionTo < 1 || s.InputVersionFrom > s.InputVersionTo {
		return fmt.Errorf("dispatch snapshot invalid input version range")
	}
	if s.Reason != "INPUT_APPENDED" && s.Reason != "EPISODE_RESOLVED" {
		return fmt.Errorf("dispatch snapshot invalid reason")
	}
	return nil
}
