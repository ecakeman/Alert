package dto

// MonitorPoint 是点位查询出参；字段与 contracts.MonitorPoint 同名同义。
type MonitorPoint struct {
	PointSeq    int64  `json:"point_seq"`
	TargetID    string `json:"target_id"`
	TargetKey   string `json:"target_key"`
	TargetType  string `json:"target_type"`
	ObservedAt  string `json:"observed_at"`
	CheckStatus string `json:"check_status"`
	StatusClass string `json:"status_class"`
	ErrorClass  string `json:"error_class"`
}

// MonitorTrendPoint 是趋势桶；桶边界与单体 dashboard 一致（整小时 UTC）。
type MonitorTrendPoint struct {
	BucketStart string `json:"bucket_start"`
	Success     int    `json:"success"`
	Failure     int    `json:"failure"`
}

// Stats 是 monitor 视角的计数结构（dashboard 目标/来源状态口径）。
type Stats struct {
	Window      string         `json:"window"`
	GeneratedAt string         `json:"generated_at"`
	Services    map[string]int `json:"services"`
	Targets     map[string]int `json:"targets"`
}

// LatestSeq 是水位线上界。
type LatestSeq struct {
	LatestSeq int64 `json:"latest_seq"`
}