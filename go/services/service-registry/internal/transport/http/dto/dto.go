package dto

import "encoding/json"

// JSON tag 与 services/contracts 及 system-core 既有对外 DTO 同名同义。

type ServiceSummary struct {
	ServiceID       string  `json:"service_id"`
	ServiceKey      string  `json:"service_key"`
	Name            string  `json:"name"`
	Description     string  `json:"description"`
	Enabled         bool    `json:"enabled"`
	MonitorStatus   string  `json:"monitor_status"`
	AppStatus       string  `json:"app_status"`
	TargetCount     int     `json:"target_count"`
	LastCheckAt     *string `json:"last_check_at"`
	ActiveEpisodeID *string `json:"active_episode_id,omitempty"`
}

type ServiceView struct {
	ServiceID              string   `json:"service_id"`
	ServiceKey             string   `json:"service_key"`
	Name                   string   `json:"name"`
	Description            string   `json:"description"`
	Enabled                bool     `json:"enabled"`
	MonitorAggregationMode string   `json:"monitor_aggregation_mode"`
	AuthorityProducerID    string   `json:"authority_producer_id"`
	MonitorStatus          string   `json:"monitor_status"`
	AppStatus              string   `json:"app_status"`
	Targets                []Target `json:"targets"`
	TargetCount            int      `json:"target_count,omitempty"`
	LastCheckAt            *string  `json:"last_check_at,omitempty"`
}

type ServicesPage struct {
	Total    int              `json:"total"`
	Limit    int              `json:"limit"`
	Offset   int              `json:"offset"`
	Services []ServiceSummary `json:"services"`
}

type ServiceCreatePayload struct {
	ServiceKey          string        `json:"service_key"`
	Name                string        `json:"name"`
	Description         string        `json:"description"`
	AuthorityProducerID string        `json:"authority_producer_id"`
	Targets             []TargetInput `json:"targets"`
}

type ServicePatchPayload struct {
	Name                   *string `json:"name"`
	Description            *string `json:"description"`
	Enabled                *bool   `json:"enabled"`
	MonitorAggregationMode *string `json:"monitor_aggregation_mode"`
}

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
	ConfigGeneration     int64           `json:"config_generation"`
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

type SourceState struct {
	ServiceID  string `json:"service_id"`
	SourceType string `json:"source_type"`
	Status     string `json:"status"`
	Reason     string `json:"reason"`
}

type ProbeResultPayload struct {
	Status               string  `json:"status"`
	ConsecutiveFailures  int     `json:"consecutive_failures"`
	ConsecutiveSuccesses int     `json:"consecutive_successes"`
	LastCheckAt          *string `json:"last_check_at"`
}

type Stats struct {
	Services int            `json:"services"`
	Targets  map[string]int `json:"targets"`
}
