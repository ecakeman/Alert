package task

import (
	"encoding/base64"
	"strconv"
	"strings"
	"time"

	"github.com/ecakeman/alert/services/system-core/internal/application/apperr"
	porttask "github.com/ecakeman/alert/services/system-core/internal/application/port/task"
	"github.com/ecakeman/alert/services/system-core/internal/domain"
)

// TaskAction 是统一 fence 的动作维度。
type TaskAction string

const (
	ActionStarted          TaskAction = "Started"
	ActionHeartbeat        TaskAction = "Heartbeat"
	ActionRegisterArtifact TaskAction = "RegisterArtifact"
	ActionReportResult     TaskAction = "ReportTaskResult"
	ActionReportFailure    TaskAction = "ReportTaskFailure"
	ActionAbandon          TaskAction = "Abandon"
	ActionGetContext       TaskAction = "GetTaskContext"
)

// actionStatuses 只是回调前的状态集合过滤，不新增状态定义。
// 状态边判定一律调用 domain.CanTransitionTask。
var actionStatuses = map[TaskAction]map[domain.TaskStatus]bool{
	ActionStarted: {
		domain.TaskPending:    true,
		domain.TaskDispatched: true,
	},
	ActionHeartbeat:        {domain.TaskRunning: true},
	ActionRegisterArtifact: {domain.TaskRunning: true},
	ActionReportResult:     {domain.TaskRunning: true},
	ActionReportFailure:    {domain.TaskRunning: true},
	ActionAbandon:          {domain.TaskRunning: true},
	ActionGetContext: {
		domain.TaskDispatched: true,
		domain.TaskRunning:    true,
	},
}

func allowsAction(act TaskAction, status domain.TaskStatus) bool {
	return actionStatuses[act][status]
}

// fence 是唯一回调校验入口：存在 → 归属 → 类型 → 代次 → 状态 → 租约。
// 拒绝立即返回稳定码，调用方不得先写后判。
func (s *Service) fence(task porttask.AgentTask, id TaskIdentity, act TaskAction, now time.Time) error {
	in := domain.FenceInput{
		Found:             task.TaskID != "",
		IncidentID:        task.EpisodeID,
		ExpectedIncident:  id.EpisodeID,
		TaskType:          task.TaskType,
		ExpectedTaskType:  id.TaskType,
		Generation:        id.Generation,
		CurrentGeneration: task.Generation,
		Status:            domain.TaskStatus(task.Status),
		// 租约放在动作-状态过滤之后单独判定，以保持六步顺序。
		LeaseValid: true,
	}
	if decision := domain.Fence(in); !decision.Accepted {
		return apperr.New(decision.Code, "task fence rejected")
	}
	if !allowsAction(act, domain.TaskStatus(task.Status)) {
		return apperr.New(domain.CodeStateConflict, "action "+string(act)+" not allowed in status "+task.Status)
	}
	if !leaseValid(task, id, now) {
		return apperr.New(domain.CodeLeaseLost, "lease token invalid or expired")
	}
	return nil
}

func leaseValid(task porttask.AgentTask, id TaskIdentity, now time.Time) bool {
	if task.LeaseUntil == nil || !task.LeaseUntil.After(now) {
		return false
	}
	taskID, generation, until, ok := decodeLeaseToken(id.LeaseToken)
	if !ok {
		return false
	}
	return taskID == task.TaskID && generation == task.Generation &&
		until > now.Unix() && task.LeaseUntil.After(now)
}

func encodeLeaseToken(taskID string, generation, leaseUntilUnix int64) string {
	raw := taskID + "|" + strconv.FormatInt(generation, 10) + "|" + strconv.FormatInt(leaseUntilUnix, 10)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeLeaseToken(token string) (string, int64, int64, bool) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return "", 0, 0, false
	}
	parts := strings.Split(string(raw), "|")
	if len(parts) != 3 {
		return "", 0, 0, false
	}
	generation, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return "", 0, 0, false
	}
	until, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return "", 0, 0, false
	}
	return parts[0], generation, until, true
}
