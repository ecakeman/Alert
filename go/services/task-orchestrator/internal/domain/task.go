package domain

// Ownership 表示一个 Service 级执行权（服务级互斥）。
type Ownership struct {
	ServiceID  string
	TaskID     string
	Generation int64
	LeaseUntil int64 // unix 秒；0 视为已过期
}

// AcquireDecision 是执行权获取结果。
type AcquireDecision struct {
	Allowed bool
	Reason  string
}

// CanAcquire：同一 Service 最多一个有效执行权；不同 Service 互不阻塞。
//   - 无持有者 / 不同 Service / 租约过期 / 同 task+generation 幂等 -> 允许
//   - 同 Service 其他 task 未过期 -> SERVICE_BUSY
func CanAcquire(existing *Ownership, req Ownership, nowUnix int64) AcquireDecision {
	if existing == nil || existing.ServiceID != req.ServiceID {
		return AcquireDecision{Allowed: true}
	}
	if existing.TaskID == req.TaskID && existing.Generation == req.Generation {
		return AcquireDecision{Allowed: true}
	}
	if existing.LeaseUntil <= nowUnix {
		return AcquireDecision{Allowed: true}
	}
	return AcquireDecision{Allowed: false, Reason: CodeServiceBusy}
}

// FenceInput 是回调/心跳/产物登记的校验输入。
type FenceInput struct {
	Found             bool
	IncidentID        string
	ExpectedIncident  string
	TaskType          string
	ExpectedTaskType  string
	Generation        int64
	CurrentGeneration int64
	Status            TaskStatus
	LeaseValid        bool
}

// FenceDecision 是 fencing 结果。
type FenceDecision struct {
	Accepted bool
	Code     string
}

// Fence 固定校验顺序：存在性 -> 归属 -> 类型 -> 代次 -> 状态 -> Lease。
// 旧代次/失租/终态一律拒绝，且拒绝码稳定、不伪装为 INTERNAL。
func Fence(in FenceInput) FenceDecision {
	if !in.Found {
		return FenceDecision{Accepted: false, Code: CodeNotFound}
	}
	if in.IncidentID != in.ExpectedIncident {
		return FenceDecision{Accepted: false, Code: CodeStateConflict}
	}
	if in.TaskType != in.ExpectedTaskType {
		return FenceDecision{Accepted: false, Code: CodeTypeMismatch}
	}
	if in.Generation != in.CurrentGeneration {
		return FenceDecision{Accepted: false, Code: CodeStaleGeneration}
	}
	if in.Status == TaskSucceeded || in.Status == TaskFailed {
		return FenceDecision{Accepted: false, Code: CodeStateConflict}
	}
	if !in.LeaseValid {
		return FenceDecision{Accepted: false, Code: CodeLeaseLost}
	}
	return FenceDecision{Accepted: true}
}

// TaskOutcome 是执行结果分类。
type TaskOutcome string

const (
	OutcomeSucceeded        TaskOutcome = "SUCCEEDED"
	OutcomeRetryableFailure TaskOutcome = "RETRYABLE_FAILURE"
	OutcomeFatalFailure     TaskOutcome = "FATAL_FAILURE"
)

// TaskEffect 只包含 Task 维度的变更；刻意不含 Incident 状态，保证结果不影响故障状态。
type TaskEffect struct {
	Status       TaskStatus
	Attempt      int
	NextRetryAt  int64
	ErrorCode    string
	ErrorMessage string
}

// ApplyTaskResult 仅产生 Task 维度变更；Incident 恢复只能走 CanResolve。
func ApplyTaskResult(outcome TaskOutcome, attempt, maxAttempts int, nowUnix, retryDelay int64) TaskEffect {
	switch outcome {
	case OutcomeSucceeded:
		return TaskEffect{Status: TaskSucceeded, Attempt: attempt}
	case OutcomeFatalFailure:
		return TaskEffect{Status: TaskFailed, Attempt: attempt + 1}
	default:
		if attempt+1 >= maxAttempts {
			return TaskEffect{Status: TaskFailed, Attempt: attempt + 1}
		}
		return TaskEffect{Status: TaskRetryWait, Attempt: attempt + 1, NextRetryAt: nowUnix + retryDelay}
	}
}

// CanAutoPromote：仅 RETRY_WAIT 且到期可自动推进；终态恒为假。
func CanAutoPromote(status TaskStatus, nextRetryAt, nowUnix int64) bool {
	return status == TaskRetryWait && nextRetryAt > 0 && nextRetryAt <= nowUnix
}

// ManualRetryPlan 是人工重试计划：终态生成新代次，非终态拒绝。
type ManualRetryPlan struct {
	OK            bool
	NewGeneration int64
	Err           error
}

func PlanManualRetry(oldGeneration int64, status TaskStatus) ManualRetryPlan {
	if status != TaskSucceeded && status != TaskFailed {
		return ManualRetryPlan{OK: false, Err: newError(CodeStateConflict, "manual retry only allowed from terminal state")}
	}
	return ManualRetryPlan{OK: true, NewGeneration: oldGeneration + 1}
}