package middleware

import (
	"log"
	"time"

	"github.com/gin-gonic/gin"
)

// AccessLog 每个请求输出一条结构化访问日志，trace_id 与 BFF 串联。
func AccessLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		log.Printf("trace_id=%s method=%s path=%s status=%d latency=%s request_id=%s",
			c.GetString("trace_id"), c.Request.Method, c.Request.URL.Path, c.Writer.Status(),
			time.Since(start), c.GetString("request_id"))
	}
}