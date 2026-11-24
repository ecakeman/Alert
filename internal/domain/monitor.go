package domain

// MONITOR 来源聚合模式（与 system.services.monitor_aggregation_mode 对应）。
const (
	ModeAllRequired = "ALL_REQUIRED"
	ModeAnyRequired = "ANY_REQUIRED"
	ModeMajority    = "MAJORITY"
)

// TargetHealth 是参与 MONITOR 聚合的单目标健康快照；纯值类型，无 IO。
type TargetHealth struct {
	Required bool
	Enabled  bool
	Deleted  bool
	Status   string
}

// AggregateMonitor 把跨必需目标的健康聚合为 MONITOR 来源状态。
//
// 参与集合：Required && Enabled && !Deleted；过滤后为空 → UNKNOWN。
// 未知模式 → CodeOutOfRange。UNKNOWN 不参与多数计数、不视为 NORMAL。
func AggregateMonitor(mode string, targets []TargetHealth) (string, error) {
	var active []TargetHealth
	for _, t := range targets {
		if t.Required && t.Enabled && !t.Deleted {
			active = append(active, t)
		}
	}
	if len(active) == 0 {
		return "UNKNOWN", nil
	}

	normal, abnormal := 0, 0
	for _, t := range active {
		switch t.Status {
		case "NORMAL":
			normal++
		case "ABNORMAL":
			abnormal++
		}
	}

	switch mode {
	case ModeAllRequired:
		if abnormal > 0 {
			return "ABNORMAL", nil
		}
		if normal == len(active) {
			return "NORMAL", nil
		}
		return "UNKNOWN", nil
	case ModeAnyRequired:
		if normal > 0 {
			return "NORMAL", nil
		}
		if abnormal == len(active) {
			return "ABNORMAL", nil
		}
		return "UNKNOWN", nil
	case ModeMajority:
		half := len(active) / 2
		if normal > half {
			return "NORMAL", nil
		}
		if abnormal > half {
			return "ABNORMAL", nil
		}
		return "UNKNOWN", nil
	default:
		return "", newError(CodeOutOfRange, "unknown monitor aggregation mode: "+mode)
	}
}
