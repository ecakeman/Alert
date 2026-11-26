package apperr

import (
	"errors"
	"net/http"

	"github.com/ecakeman/alert/services/task-orchestrator/internal/domain"
)

// Normalize 把 domain 稳定错误归一为同 code 的 apperr.Error；非 domain 错误原样返回。
func Normalize(err error) error {
	if err == nil {
		return nil
	}
	var de *domain.Error
	if errors.As(err, &de) {
		return &Error{Code: de.Code, Message: de.Message}
	}
	return err
}

// Code 提取 apperr 稳定码；非 apperr 错误返回 INTERNAL。
func Code(err error) string {
	var ae *Error
	if errors.As(err, &ae) {
		return ae.Code
	}
	return "INTERNAL"
}

// HTTPStatus 返回稳定业务码对应的 HTTP 状态（400/404/409/501/503/500 六档）。
// domain 稳定码经 Normalize 归一后沿用原映射，状态不变。
func HTTPStatus(code string) int {
	switch code {
	case CodeValidation:
		return http.StatusBadRequest
	case CodeNotFound, CodeArtifactNotFound:
		return http.StatusNotFound
	case CodeAlreadyExists, CodeEventIDConflict:
		return http.StatusConflict
	case CodeNotImplemented:
		return http.StatusNotImplemented
	case CodeDependencyUnavailable:
		return http.StatusServiceUnavailable
	case domain.CodeInvalidRange, domain.CodeOutOfRange, domain.CodeCrossIncident, domain.CodeUnknownArtifact:
		return http.StatusBadRequest
	// NOT_FOUND / EVENT_ID_CONFLICT 与 apperr 同码，已由上方 case 覆盖（404/409）。
	case domain.CodeInvalidTransition, domain.CodeStateConflict, domain.CodeStaleGeneration,
		domain.CodeLeaseLost, domain.CodeServiceBusy,
		domain.CodeHashMismatch, domain.CodeNotResolved, domain.CodeOwnerMismatch,
		domain.CodeTypeMismatch, domain.CodeArtifactMissing:
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}
