package dto

type DashboardSummary struct {
	GeneratedAt    string              `json:"generated_at"`
	Services       map[string]int      `json:"services"`
	Targets        map[string]int      `json:"targets"`
	Episodes       map[string]int      `json:"episodes"`
	RecentEpisodes []EpisodeSummary    `json:"recent_episodes"`
	MonitorTrend   []MonitorTrendPoint `json:"monitor_trend"`
}

type MonitorTrendPoint struct {
	BucketStart string `json:"bucket_start"`
	Success     int    `json:"success"`
	Failure     int    `json:"failure"`
}
