package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/ecakeman/alert/services/system-core/internal/transport/http/httperr"
	"github.com/ecakeman/alert/services/system-core/internal/transport/http/response"
)

// NotImplemented 是骨架阶段所有未实现业务路由的统一 501 响应。
func NotImplemented(c *gin.Context) {
	status, code, msg := httperr.NotImplemented()
	response.Error(c, status, code, msg)
}
