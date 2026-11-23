package httperr

import (
	"errors"
	"net/http"

	"github.com/ecakeman/alert/internal/application/apperr"
	"github.com/ecakeman/alert/internal/domain"
)

// Status 把错误映射为 HTTP 状态码（400/404/409/501/503/500 六档）。
func Status(err error) int {
	if err == nil {
		return http.StatusOK
	}

	var ae *apperr.Error
	if errors.As(err, &ae) {
		switch ae.Code {
		case apperr.CodeValidation:
			return http.StatusBadRequest
		case apperr.CodeNotFound, apperr.CodeArtifactNotFound:
			return http.StatusNotFound
		case apperr.CodeAlreadyExists:
			return http.StatusConflict
		case apperr.CodeNotImplemented:
			return http.StatusNotImplemented
		case apperr.CodeDependencyUnavailable:
			return http.StatusServiceUnavailable
		default:
			return http.StatusInternalServerError
		}
	}

	var de *domain.Error
	if errors.As(err, &de) {
		switch de.Code {
		case domain.CodeInvalidRange, domain.CodeOutOfRange, domain.CodeCrossIncident, domain.CodeUnknownArtifact:
			return http.StatusBadRequest
		case domain.CodeNotFound:
			return http.StatusNotFound
		case domain.CodeInvalidTransition, domain.CodeStateConflict, domain.CodeStaleGeneration,
			domain.CodeLeaseLost, domain.CodeServiceBusy, domain.CodeEventIDConflict,
			domain.CodeHashMismatch, domain.CodeNotResolved, domain.CodeOwnerMismatch,
			domain.CodeTypeMismatch, domain.CodeArtifactMissing:
			return http.StatusConflict
		default:
			return http.StatusInternalServerError
		}
	}
	return http.StatusInternalServerError
}

// Classify 返回状态码、稳定 code、可读 message；未知错误统一 INTERNAL。
func Classify(err error) (int, string, string) {
	status := Status(err)

	var ae *apperr.Error
	if errors.As(err, &ae) {
		return status, ae.Code, ae.Message
	}
	var de *domain.Error
	if errors.As(err, &de) {
		return status, de.Code, de.Message
	}
	return status, "INTERNAL", "internal error"
}

// NotImplemented 是骨架阶段未实现路由的统一错误。
func NotImplemented() (int, string, string) {
	return http.StatusNotImplemented, apperr.CodeNotImplemented, "not implemented in skeleton"
}