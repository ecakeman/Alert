package mapper

import (
	apptask "github.com/ecakeman/alert/services/system-core/internal/application/task"
	"github.com/ecakeman/alert/services/system-core/internal/transport/http/dto"
)

// ToTaskAccepted 映射人工重试的异步受理视图。
func ToTaskAccepted(view apptask.TaskView, reused bool) dto.TaskAccepted {
	return dto.TaskAccepted{
		TaskID:     view.TaskID,
		EpisodeID:  view.EpisodeID,
		TaskType:   view.TaskType,
		Status:     view.Status,
		Generation: view.Generation,
		Attempt:    view.Attempt,
		CreatedAt:  view.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		Reused:     reused,
	}
}
