package apierr

import (
	"encoding/json"
	"errors"
	"net/http"
)

// Error 是跨服务稳定业务错误；不依赖任何域实体或框架。
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// 7 类稳定错误码。
const (
	CodeValidation            = "VALIDATION"
	CodeNotFound              = "NOT_FOUND"
	CodeAlreadyExists         = "ALREADY_EXISTS"
	CodeStateConflict         = "STATE_CONFLICT"
	CodeDependencyUnavailable = "DEPENDENCY_UNAVAILABLE"
	CodeNotImplemented        = "NOT_IMPLEMENTED"
	CodeInternal              = "INTERNAL"
)

func New(code, message string) *Error { return &Error{Code: code, Message: message} }

func Validation(message string) *Error            { return New(CodeValidation, message) }
func NotFound(message string) *Error              { return New(CodeNotFound, message) }
func AlreadyExists(message string) *Error         { return New(CodeAlreadyExists, message) }
func StateConflict(message string) *Error         { return New(CodeStateConflict, message) }
func DependencyUnavailable(message string) *Error { return New(CodeDependencyUnavailable, message) }
func NotImplemented(message string) *Error        { return New(CodeNotImplemented, message) }

// HTTPStatus 返回稳定码对应的 HTTP 状态：422/404/409/503/501/500。
func HTTPStatus(code string) int {
	switch code {
	case CodeValidation:
		return http.StatusUnprocessableEntity
	case CodeNotFound:
		return http.StatusNotFound
	case CodeAlreadyExists, CodeStateConflict:
		return http.StatusConflict
	case CodeDependencyUnavailable:
		return http.StatusServiceUnavailable
	case CodeNotImplemented:
		return http.StatusNotImplemented
	default:
		return http.StatusInternalServerError
	}
}

// Classify 把任意错误归一为稳定码与可读消息；非业务错误不泄露原始文本。
func Classify(err error) (int, *Error) {
	var ae *Error
	if errors.As(err, &ae) {
		return HTTPStatus(ae.Code), ae
	}
	return http.StatusInternalServerError, New(CodeInternal, "internal error")
}

// Write 按稳定码写 JSON 响应；err 为空时写 204。
func Write(w http.ResponseWriter, err error) {
	if err == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	status, ae := Classify(err)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ae)
}
