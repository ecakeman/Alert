package middleware

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
)

// Timeout 为请求上下文设置超时上限。
func Timeout(d time.Duration) gin.HandlerFunc {
	if d <= 0 {
		d = 10 * time.Second
	}
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), d)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}