package task

import (
	"context"
	"time"

	"github.com/ecakeman/alert/services/task-orchestrator/internal/application/apperr"
	portshared "github.com/ecakeman/alert/services/task-orchestrator/internal/application/port/shared"
	"github.com/ecakeman/alert/services/task-orchestrator/internal/domain"
)

// TaskContext 是执行器读取的冻结上下文视图。
type TaskContext struct {
	InputVersionFrom  int64
	InputVersionTo    int64
	MonitorWatermark  int64
	AppEventWatermark int64
	DeadlineAt        time.Time
	TaskType          string
}

// GetContext 校验任务身份与冻结范围后返回上下文；越界返回 OUT_OF_RANGE。
func (s *Service) GetContext(ctx context.Context, id TaskIdentity, from, to, watermark int64) (TaskContext, error) {
	var out TaskContext
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		task, err := s.deps.Tasks.GetForUpdate(ctx, tx, id.TaskID)
		if err != nil {
			return err
		}
		if err := s.fence(task, id, ActionGetContext, s.deps.Clock.Now()); err != nil {
			return err
		}
		if from < task.InputVersionFrom || to > task.InputVersionTo || from > to || watermark > task.InputVersionTo {
			return apperr.New(domain.CodeOutOfRange, "requested range is outside the frozen bound")
		}
		out = TaskContext{
			InputVersionFrom:  task.InputVersionFrom,
			InputVersionTo:    task.InputVersionTo,
			MonitorWatermark:  task.MonitorWatermark,
			AppEventWatermark: task.AppEventWatermark,
			DeadlineAt:        task.DeadlineAt,
			TaskType:          task.TaskType,
		}
		return nil
	})
	return out, err
}
