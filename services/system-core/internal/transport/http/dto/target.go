package dto

import "encoding/json"

type Target struct {
	TargetID             string          `json:"target_id"`
	TargetKey            string          `json:"target_key"`
	TargetName           string          `json:"target_name"`
	TargetType           string          `json:"target_type"`
	Address              string          `json:"address"`
	Config               json.RawMessage `json:"config"`
	IntervalMS           int             `json:"interval_ms"`
	TimeoutMS            int             `json:"timeout_ms"`
	FailureThreshold     int             `json:"failure_threshold"`
	RecoveryThreshold    int             `json:"recovery_threshold"`
	IsRequired           bool            `json:"is_required"`
	Enabled              bool            `json:"enabled"`
	Status               string          `json:"status"`
	ConsecutiveFailures  int             `json:"consecutive_failures"`
	ConsecutiveSuccesses int             `json:"consecutive_successes"`
	LastCheckAt          *string         `json:"last_check_at"`
}

type TargetInput struct {
	TargetKey         string          `json:"target_key"`
	TargetName        string          `json:"target_name"`
	TargetType        string          `json:"target_type"`
	Address           string          `json:"address"`
	Config            json.RawMessage `json:"config"`
	IntervalMS        int             `json:"interval_ms"`
	TimeoutMS         int             `json:"timeout_ms"`
	FailureThreshold  int             `json:"failure_threshold"`
	RecoveryThreshold int             `json:"recovery_threshold"`
	IsRequired        bool            `json:"is_required"`
	Enabled           bool            `json:"enabled"`
}

type TargetPatchPayload struct {
	TargetName        *string          `json:"target_name"`
	Address           *string          `json:"address"`
	Config            *json.RawMessage `json:"config"`
	IntervalMS        *int             `json:"interval_ms"`
	TimeoutMS         *int             `json:"timeout_ms"`
	FailureThreshold  *int             `json:"failure_threshold"`
	RecoveryThreshold *int             `json:"recovery_threshold"`
	IsRequired        *bool            `json:"is_required"`
	Enabled           *bool            `json:"enabled"`
}

// TargetsResponse 是 GET /services/:id/targets 的响应体。
type TargetsResponse struct {
	Targets []Target `json:"targets"`
}
