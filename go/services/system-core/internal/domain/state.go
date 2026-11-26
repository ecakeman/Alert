package domain

// IncidentStatus 对应 system.episodes.status 的取值域。
type IncidentStatus string

const (
	IncidentActive   IncidentStatus = "ACTIVE"
	IncidentResolved IncidentStatus = "RESOLVED"
	IncidentArchived IncidentStatus = "ARCHIVED"
)

// TaskStatus 对应 system.agent_tasks.status 的取值域。
type TaskStatus string

const (
	TaskPending    TaskStatus = "PENDING"
	TaskDispatched TaskStatus = "DISPATCHED"
	TaskRunning    TaskStatus = "RUNNING"
	TaskRetryWait  TaskStatus = "RETRY_WAIT"
	TaskSucceeded  TaskStatus = "SUCCEEDED"
	TaskFailed     TaskStatus = "FAILED"
)

// 合法的状态边。未登记的边一律拒绝（含 ACTIVE->ARCHIVED、终态回流）。
var incidentTransitions = map[IncidentStatus]map[IncidentStatus]bool{
	IncidentActive:   {IncidentResolved: true},
	IncidentResolved: {IncidentArchived: true},
}

var taskTransitions = map[TaskStatus]map[TaskStatus]bool{
	TaskPending:    {TaskDispatched: true, TaskRunning: true},
	TaskDispatched: {TaskRunning: true},
	TaskRunning:    {TaskSucceeded: true, TaskRetryWait: true, TaskFailed: true},
	TaskRetryWait:  {TaskDispatched: true},
}

// CanTransitionIncident 校验故障实例状态转换；同态视为无操作。
func CanTransitionIncident(from, to IncidentStatus) error {
	if from == to {
		return nil
	}
	if incidentTransitions[from][to] {
		return nil
	}
	return newError(CodeInvalidTransition, "incident "+string(from)+" -> "+string(to)+" not allowed")
}

// CanTransitionTask 校验任务状态转换；同态视为无操作。
func CanTransitionTask(from, to TaskStatus) error {
	if from == to {
		return nil
	}
	if taskTransitions[from][to] {
		return nil
	}
	return newError(CodeInvalidTransition, "task "+string(from)+" -> "+string(to)+" not allowed")
}