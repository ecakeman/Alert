package aggregate

import (
	"encoding/json"
	"fmt"
)

// TargetL1 是 target 的对外 L1 形状（14 键，无 config_generation）。
type TargetL1 struct {
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

// ServiceViewL1 是 03/04/05 的对外 L1 形状（10 基础键 + 3 可选键）。
type ServiceViewL1 struct {
	ServiceID              string     `json:"service_id"`
	ServiceKey             string     `json:"service_key"`
	Name                   string     `json:"name"`
	Description            string     `json:"description"`
	Enabled                bool       `json:"enabled"`
	MonitorAggregationMode string     `json:"monitor_aggregation_mode"`
	AuthorityProducerID    string     `json:"authority_producer_id"`
	MonitorStatus          string     `json:"monitor_status"`
	AppStatus              string     `json:"app_status"`
	Targets                []TargetL1 `json:"targets"`
	TargetCount            int        `json:"target_count,omitempty"`
	LastCheckAt            *string    `json:"last_check_at,omitempty"`
	ActiveEpisodeID        *string    `json:"active_episode_id,omitempty"`
}

// TargetsListL1 是 07 的对外 L1 形状 {"targets":[...]}，空列表输出 []。
type TargetsListL1 struct {
	Targets []TargetL1 `json:"targets"`
}

// ServiceSummaryL1 为 P6.3 列表项预留：target_count/last_check_at 无 omitempty。
type ServiceSummaryL1 struct {
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

func projectTargetL1(raw []byte) ([]byte, error) {
	var target TargetL1
	if err := json.Unmarshal(raw, &target); err != nil {
		return nil, fmt.Errorf("decode target l1: %w", err)
	}
	return json.Marshal(target)
}

func projectTargetsListL1(raw []byte) ([]byte, error) {
	var targets []TargetL1
	if err := json.Unmarshal(raw, &targets); err != nil {
		return nil, fmt.Errorf("decode targets list l1: %w", err)
	}
	if targets == nil {
		targets = []TargetL1{}
	}
	return json.Marshal(TargetsListL1{Targets: targets})
}

func projectServiceViewL1(raw []byte, active map[string]string) ([]byte, error) {
	var view ServiceViewL1
	if err := json.Unmarshal(raw, &view); err != nil {
		return nil, fmt.Errorf("decode service view l1: %w", err)
	}
	if view.Targets == nil {
		view.Targets = []TargetL1{}
	}
	if active != nil {
		if id, ok := active[view.ServiceID]; ok && id != "" {
			view.ActiveEpisodeID = &id
		}
	}
	return json.Marshal(view)
}