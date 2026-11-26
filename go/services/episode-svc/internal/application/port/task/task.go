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

// ArtifactReadiness 与 contracts.ArtifactReadiness 同形（派发快照字段）。
type ArtifactReadiness struct {
	Investigation   bool
	DiagnosisReport bool
	HashesMatch     bool
}

// Reader 是 episode 域对 task 域的只读端口；调用方必须在事务外使用。
// 无活动任务返回 (nil, nil)，下游不可达返回 DependencyUnavailable。
type Reader interface {
	GetActive(ctx context.Context, episodeID string) (*ActiveTask, error)
	GetArtifactReadiness(ctx context.Context, episodeID string) (ArtifactReadiness, error)
}