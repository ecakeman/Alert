package dto

// TargetsResponse 是 GET /services/:id/targets 的响应体。
type TargetsResponse struct {
	Targets []Target `json:"targets"`
}
