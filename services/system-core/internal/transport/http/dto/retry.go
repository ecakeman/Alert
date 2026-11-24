package dto

// TaskAccepted 是人工重试路由的异步受理响应。
type TaskAccepted struct {
	TaskID     string `json:"task_id"`
	EpisodeID  string `json:"episode_id"`
	TaskType   string `json:"task_type"`
	Status     string `json:"status"`
	Generation int64  `json:"generation"`
	Attempt    int    `json:"attempt"`
	CreatedAt  string `json:"created_at"`
	Reused     bool   `json:"reused"`
}
