package middleware

import (
	"log"
	"time"

	"github.com/gin-gonic/gin"
)

// AccessLog 每个请求输出一条结构化访问日志。
func AccessLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		log.Printf("method=%s path=%s status=%d latency=%s request_id=%s",
			c.Request.Method, c.Request.URL.Path, c.Writer.Status(),
			time.Since(start), c.GetString("request_id"))
	}
}