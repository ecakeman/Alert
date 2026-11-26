package task

import "context"

// ActiveTask 是解除判定所需的“无活动任务”条件输入（与 contracts.ActiveTask 同形）。
type ActiveTask struct {
	TaskID     string
	EpisodeID  string
	ServiceID  string
	TaskType   string
	Status     string
	Generation int64
}

// Reader 是 episode 域对 task 域的只读端口；调用方必须在事务外使用。
// 无活动任务返回 (nil, nil)，下游不可达返回 DependencyUnavailable。
type Reader interface {
	GetActive(ctx context.Context, episodeID string) (*ActiveTask, error)
}