package aggregate

// Dashboard 对齐 P0 基线 01_dashboard_summary.json 的顶层形状。
type Dashboard struct {
	Episodes       EpisodeCounts   `json:"episodes"`
	GeneratedAt    string          `json:"generated_at"`
	MonitorTrend   []TrendPoint    `json:"monitor_trend"`
	RecentEpisodes []RecentEpisode `json:"recent_episodes"`
	Services       ServiceCounts   `json:"services"`
	Targets        TargetCounts    `json:"targets"`
}

type EpisodeCounts struct {
	Active   int `json:"active"`
	Archived int `json:"archived"`
	Resolved int `json:"resolved"`
}

type ServiceCounts struct {
	Abnormal int `json:"abnormal"`
	Disabled int `json:"disabled"`
	Enabled  int `json:"enabled"`
	Normal   int `json:"normal"`
	Total    int `json:"total"`
	Unknown  int `json:"unknown"`
}

type TargetCounts struct {
	Abnormal int `json:"abnormal"`
	Normal   int `json:"normal"`
	Total    int `json:"total"`
	Unknown  int `json:"unknown"`
}

type TrendPoint struct {
	At     string `json:"at"`
	Status string `json:"status"`
}

// RecentEpisode 对齐基线列表项；P5.10 先填本地可得字段，P6.3 补齐跨域。
type RecentEpisode struct {
	ActiveTaskStatus      string  `json:"active_task_status"`
	ActiveTaskType        string  `json:"active_task_type"`
	ArchivedAt            *string `json:"archived_at"`
	EpisodeID             string  `json:"episode_id"`
	HasDiagnosisReport    bool    `json:"has_diagnosis_report"`
	InputVersion          int64   `json:"input_version"`
	ProcessedInputVersion int64   `json:"processed_input_version"`
	ResolutionPending     bool    `json:"resolution_pending"`
	ResolvedAt            *string `json:"resolved_at"`
	ServiceID             string  `json:"service_id"`
	ServiceName           string  `json:"service_name"`
	StartedAt             string  `json:"started_at"`
	StartedBy             string  `json:"started_by"`
	Status                string  `json:"status"`
	UpdatedAt             string  `json:"updated_at"`
}