package postgres

import (
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/ecakeman/alert/services/task-orchestrator/internal/application/apperr"
	"github.com/ecakeman/alert/services/task-orchestrator/internal/domain"
)

// 唯一约束名 → 稳定业务码（约束名由 B1 固定）。
var uniqueConstraints = map[string]string{
	"services_service_key_key":            apperr.CodeAlreadyExists,
	"service_monitor_targets_active_key":  apperr.CodeAlreadyExists,
	"episodes_one_active":                 apperr.CodeAlreadyExists,
	"episode_input_versions_hash":         apperr.CodeAlreadyExists,
	"episode_app_events_identity":         apperr.CodeAlreadyExists,
	"episode_app_events_seq":              apperr.CodeAlreadyExists,
	"agent_tasks_one_active":              apperr.CodeAlreadyExists,
	"service_execution_ownership_pkey":    apperr.CodeAlreadyExists,
	"outbox_events_dedupe_key":            apperr.CodeAlreadyExists,
	"consumer_idempotency_pkey":           apperr.CodeAlreadyExists,
	"episode_artifact_refs_version":       apperr.CodeAlreadyExists,
	"episode_fault_document_refs_version": apperr.CodeAlreadyExists,
}

// Translate 把 pg 错误翻译为稳定业务错误；不泄露 SQL / 参数 / 连接串。
func Translate(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return apperr.NotFound("resource not found")
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		if _, ok := uniqueConstraints[pgErr.ConstraintName]; ok {
			return apperr.AlreadyExists("resource already exists")
		}
		switch pgErr.Code {
		case "23505":
			return apperr.AlreadyExists("resource already exists")
		case "23503":
			return &domain.Error{Code: domain.CodeStateConflict, Message: "related resource constraint"}
		case "23514":
			return apperr.Validation("constraint violated")
		}
	}
	return apperr.New("INTERNAL", "internal error")
}

// NoRows 判断是否为无结果，供仓储在条件更新未命中的场景分流。
func NoRows(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}
