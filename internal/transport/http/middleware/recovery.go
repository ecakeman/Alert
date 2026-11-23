package middleware

import (
	"github.com/gin-gonic/gin"

	"github.com/ecakeman/alert/internal/transport/http/response"
)

// Recovery 捕获 panic，返回 500 INTERNAL，响应体不含堆栈。
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				response.Error(c, 500, "INTERNAL", "internal error")
				c.Abort()
			}
		}()
		c.Next()
	}
}