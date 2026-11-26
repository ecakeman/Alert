package apperr

import (
	"errors"
	"net/http"
)

// Error 是应用层稳定业务错误；不依赖 domain / gin / pgx。
type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// 应用层稳定错误码。
const (
	CodeValidation            = "VALIDATION"
	CodeNotFound              = "NOT_FOUND"
	CodeAlreadyExists         = "ALREADY_EXISTS"
	CodeNotImplemented        = "NOT_IMPLEMENTED"
	CodeDependencyUnavailable = "DEPENDENCY_UNAVAILABLE"
	CodeArtifactNotFound      = "ARTIFACT_NOT_FOUND"
	CodeEventIDConflict       = "EVENT_ID_CONFLICT"
	CodeInternal              = "INTERNAL"
)

func New(code, message string) *Error { return &Error{Code: code, Message: message} }

func Validation(message string) *Error            { return New(CodeValidation, message) }
func NotFound(message string) *Error              { return New(CodeNotFound, message) }
func AlreadyExists(message string) *Error         { return New(CodeAlreadyExists, message) }
func NotImplemented(message string) *Error        { return New(CodeNotImplemented, message) }
func DependencyUnavailable(message string) *Error { return New(CodeDependencyUnavailable, message) }
func ArtifactNotFound(message string) *Error      { return New(CodeArtifactNotFound, message) }
// Code 提取稳定错误码；非业务错误返回 INTERNAL。
func Code(err error) string {
	var ae *Error
	if errors.As(err, &ae) {
		return ae.Code
	}
	return CodeInternal
}

// HTTPStatus 返回稳定码对应的 HTTP 状态：422/404/409/503/501/500。
func HTTPStatus(code string) int {
	switch code {
	case CodeValidation:
		return http.StatusUnprocessableEntity
	case CodeNotFound, CodeArtifactNotFound:
		return http.StatusNotFound
	case CodeAlreadyExists, CodeEventIDConflict:
		return http.StatusConflict
	case CodeDependencyUnavailable:
		return http.StatusServiceUnavailable
	case CodeNotImplemented:
		return http.StatusNotImplemented
	default:
		return http.StatusInternalServerError
	}
}