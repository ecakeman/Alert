package postgres

import (
	"context"
	"time"

	portshared "github.com/ecakeman/alert/internal/application/port/shared"
	porttask "github.com/ecakeman/alert/internal/application/port/task"
	"github.com/ecakeman/alert/internal/infrastructure/persistence/postgres/model"
)

type taskRepo struct{}

func NewTaskRepo() porttask.TaskRepo { return taskRepo{} }

func (taskRepo) Create(ctx context.Context, tx portshared.Tx, t porttask.AgentTask) error {
	row := model.TaskRowFromApp(t)
	_, err := txFrom(tx).Exec(ctx, `
INSERT INTO system.agent_tasks(task_id,episode_id,service_id,task_type,status,generation,attempt,max_attempts,lease_until,next_retry_at,input_version_from,input_version_to,monitor_watermark,app_event_watermark,deadline_at,error_code,error_message,created_at,updated_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)`,
		row.TaskID, row.EpisodeID, row.ServiceID, row.TaskType, row.Status, row.Generation,
		row.Attempt, row.MaxAttempts, row.LeaseUntil, row.NextRetryAt,
		row.InputVersionFrom, row.InputVersionTo, row.MonitorWatermark, row.AppEventWatermark,
		utc(row.DeadlineAt), row.ErrorCode, row.ErrorMessage, utc(row.CreatedAt), utc(row.UpdatedAt))
	return Translate(err)
}

func (taskRepo) Get(ctx context.Context, tx portshared.Tx, taskID string) (porttask.AgentTask, error) {
	row, err := model.TaskRow{}.Scan(txFrom(tx).QueryRow(ctx,
		`SELECT `+model.TaskColumns+` FROM system.agent_tasks WHERE task_id=$1`, taskID))
	if err != nil {
		return porttask.AgentTask{}, Translate(err)
	}
	return row.ToApp(), nil
}

func (taskRepo) ListActiveByEpisode(ctx context.Context, tx portshared.Tx, episodeID string) ([]porttask.AgentTask, error) {
	rows, err := txFrom(tx).Query(ctx, `
SELECT `+model.TaskColumns+` FROM system.agent_tasks
WHERE episode_id=$1 AND status IN ('PENDING','DISPATCHED','RUNNING','RETRY_WAIT')
ORDER BY created_at DESC`, episodeID)
	if err != nil {
		return nil, Translate(err)
	}
	defer rows.Close()
	var out []porttask.AgentTask
	for rows.Next() {
		row, err := model.TaskRow{}.Scan(rows)
		if err != nil {
			return nil, Translate(err)
		}
		out = append(out, row.ToApp())
	}
	return out, Translate(rows.Err())
}

// UpdateCAS：以 task_id + generation + 允许状态 + 有效租约为条件，返回受影响行数。
func (taskRepo) UpdateCAS(ctx context.Context, tx portshared.Tx, taskID string, generation int64, expectedStatuses []string, patch porttask.TaskPatch) (int64, error) {
	tag, err := txFrom(tx).Exec(ctx, `
UPDATE system.agent_tasks SET
  status = COALESCE($4, status),
  lease_until = COALESCE($5, lease_until),
  next_retry_at = COALESCE($6, next_retry_at),
  attempt = COALESCE($7, attempt),
  error_code = COALESCE($8, error_code),
  error_message = COALESCE($9, error_message),
  started_at = COALESCE($10, started_at),
  finished_at = COALESCE($11, finished_at),
  updated_at = now()
WHERE task_id=$1 AND generation=$2 AND status = ANY($3)
  AND (lease_until IS NULL OR lease_until > now())`,
		taskID, generation, expectedStatuses, patch.Status, patch.LeaseUntil, patch.NextRetryAt,
		patch.Attempt, patch.ErrorCode, patch.ErrorMessage, patch.StartedAt, patch.FinishedAt)
	if err != nil {
		return 0, Translate(err)
	}
	return tag.RowsAffected(), nil
}

func (taskRepo) ListDueRetry(ctx context.Context, tx portshared.Tx, now time.Time, limit int) ([]porttask.AgentTask, error) {
	if limit <= 0 {
		limit = 32
	}
	rows, err := txFrom(tx).Query(ctx, `
SELECT `+model.TaskColumns+` FROM system.agent_tasks
WHERE status='RETRY_WAIT' AND next_retry_at IS NOT NULL AND next_retry_at <= $1
ORDER BY next_retry_at LIMIT $2`, utc(now), limit)
	if err != nil {
		return nil, Translate(err)
	}
	defer rows.Close()
	var out []porttask.AgentTask
	for rows.Next() {
		row, err := model.TaskRow{}.Scan(rows)
		if err != nil {
			return nil, Translate(err)
		}
		out = append(out, row.ToApp())
	}
	return out, Translate(rows.Err())
}

func (taskRepo) ListDispatchPending(ctx context.Context, tx portshared.Tx, limit int) ([]porttask.AgentTask, error) {
	if limit <= 0 {
		limit = 32
	}
	rows, err := txFrom(tx).Query(ctx, `
SELECT `+model.TaskColumns+` FROM system.agent_tasks
WHERE status='PENDING' ORDER BY created_at LIMIT $1`, limit)
	if err != nil {
		return nil, Translate(err)
	}
	defer rows.Close()
	var out []porttask.AgentTask
	for rows.Next() {
		row, err := model.TaskRow{}.Scan(rows)
		if err != nil {
			return nil, Translate(err)
		}
		out = append(out, row.ToApp())
	}
	return out, Translate(rows.Err())
}
