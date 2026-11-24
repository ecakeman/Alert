package httperr

import (
	"errors"
	"net/http"

	"github.com/ecakeman/alert/internal/application/apperr"
)

// Status 把错误映射为 HTTP 状态码（400/404/409/501/503/500 六档）。
// domain 错误经 apperr.Normalize 归一后沿用原映射，状态不变。
func Status(err error) int {
	if err == nil {
		return http.StatusOK
	}
	return apperr.HTTPStatus(apperr.Code(apperr.Normalize(err)))
}

// Classify 返回状态码、稳定 code、可读 message；未知错误统一 INTERNAL。
func Classify(err error) (int, string, string) {
	status := Status(err)
	var ae *apperr.Error
	if errors.As(apperr.Normalize(err), &ae) {
		return status, ae.Code, ae.Message
	}
	return status, "INTERNAL", "internal error"
}

// NotImplemented 是骨架阶段未实现路由的统一错误。
func NotImplemented() (int, string, string) {
	return http.StatusNotImplemented, apperr.CodeNotImplemented, "not implemented in skeleton"
}
