package task

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/ecakeman/alert/services/system-core/internal/application/apperr"
	portshared "github.com/ecakeman/alert/services/system-core/internal/application/port/shared"
	"github.com/ecakeman/alert/services/system-core/internal/contracts"
)

type dispatchPayload struct {
	TaskID     string `json:"task_id"`
	Generation int64  `json:"generation"`
}

// HandleDispatchEvent 解码派发事件后执行 Dispatch；重复投递零副作用，失败不 MarkDone。
func (s *Service) HandleDispatchEvent(ctx context.Context, event contracts.Event) error {
	if event.EventType != contracts.EventTaskDispatchRequested {
		return nil
	}
	var payload dispatchPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		log.Printf("WARN skip dispatch event event_id=%s reason=%v", event.EventID, err)
		return nil
	}
	if payload.TaskID == "" || payload.Generation <= 0 {
		log.Printf("WARN skip dispatch event event_id=%s reason=invalid payload", event.EventID)
		return nil
	}
	result, err := s.dispatch(ctx, payload.TaskID, payload.Generation, event.EventID)
	if err != nil {
		log.Printf("ERROR dispatch event failed event_id=%s task_id=%s reason=%v", event.EventID, payload.TaskID, err)
		return err
	}
	return s.markDispatchDone(ctx, event.EventID, payload.TaskID, payload.Generation, string(result))
}

func (s *Service) markDispatchDone(ctx context.Context, eventID, taskID string, generation int64, resultCode string) error {
	return s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		now := s.deps.Clock.Now()
		for _, key := range []string{"event:" + eventID, fmt.Sprintf("taskgen:%s:%d", taskID, generation)} {
			if err := s.deps.Idem.MarkDone(ctx, tx, dispatchConsumer, key, resultCode, now); err != nil {
				return apperr.Normalize(err)
			}
		}
		return nil
	})
}

// HandleDispatchableEvent 是派发主路径：直接用 episode 快照建任务，零回查。
// 重复投递由 Create 内的 episode 幂等键拦截；失败返回错误由消费者重试。
func (s *Service) HandleDispatchableEvent(ctx context.Context, event contracts.Event) error {
	if event.EventType != contracts.EventEpisodeDispatchable {
		return nil
	}
	var snapshot contracts.DispatchSnapshot
	if err := json.Unmarshal(event.Payload, &snapshot); err != nil {
		return fmt.Errorf("decode dispatchable payload: %w", err)
	}
	// 未知快照字段不能阻塞建任务；reason 决定任务类型。
	taskType := TaskTypeDiagnosis
	source := SourceInputAppended
	if snapshot.Reason == "EPISODE_RESOLVED" {
		taskType = TaskTypeArchive
		source = SourceEpisodeResolved
	}
	_, _, err := s.Create(ctx, CreateInput{Snapshot: snapshot, TaskType: taskType, Source: source})
	return err
}
