package domain

// IncidentDecisionKind 表示面对异常输入时的建/复用决策。
type IncidentDecisionKind int

const (
	IncidentReuse IncidentDecisionKind = iota
	IncidentCreate
)

type IncidentDecision struct {
	Kind       IncidentDecisionKind
	IncidentID string
}

// DecideIncidentOnAbnormal 在“输入已判定为异常”之后决定复用现有 ACTIVE 还是新建。
// 真实并发唯一性由数据库约束兜底，本函数不负责并发。
func DecideIncidentOnAbnormal(activeExists bool, activeID string) IncidentDecision {
	if activeExists && activeID != "" {
		return IncidentDecision{Kind: IncidentReuse, IncidentID: activeID}
	}
	return IncidentDecision{Kind: IncidentCreate}
}

// ResolveInput 是故障解除判定所需的输入条件。
// ResolutionPending 仅作中间态，不参与“是否已 RESOLVED”的最终判定。
type ResolveInput struct {
	MonitorNormal       bool
	AppNormal           bool
	FinalInputProcessed bool
	ActiveTaskExists    bool
	ResolutionPending   bool
}

// MissingCondition 描述未满足的解除条件。
type MissingCondition string

const (
	MissingMonitorNormal       MissingCondition = "MONITOR_NOT_NORMAL"
	MissingAppNormal           MissingCondition = "APP_NOT_NORMAL"
	MissingFinalInputProcessed MissingCondition = "FINAL_INPUT_NOT_PROCESSED"
	MissingNoActiveTask        MissingCondition = "ACTIVE_TASK_EXISTS"
)

// CanResolve 返回是否满足全部解除条件，以及全部缺失项（便于解释“为什么没恢复”）。
func CanResolve(in ResolveInput) (bool, []MissingCondition) {
	var missing []MissingCondition
	if !in.MonitorNormal {
		missing = append(missing, MissingMonitorNormal)
	}
	if !in.AppNormal {
		missing = append(missing, MissingAppNormal)
	}
	if !in.FinalInputProcessed {
		missing = append(missing, MissingFinalInputProcessed)
	}
	if in.ActiveTaskExists {
		missing = append(missing, MissingNoActiveTask)
	}
	return len(missing) == 0, missing
}
