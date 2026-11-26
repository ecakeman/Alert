package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"regexp"

	"github.com/gin-gonic/gin"
)

// HeaderTraceID 与 BFF / platform 保持同名；单体直连时用 request_id 兜底。
const HeaderTraceID = "X-Trace-Id"

var traceIDRe = regexp.MustCompile(`^[0-9a-f]{32}$`)

// RequestID 生成或透传 X-Request-ID，同时解析/生成 X-Trace-Id：
// BFF 已注入合法 trace 时原样沿用，否则以 request_id 作为 trace。
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		rid := c.GetHeader("X-Request-ID")
		if rid == "" {
			rid = newRequestID()
		}
		traceID := c.GetHeader(HeaderTraceID)
		if !traceIDRe.MatchString(traceID) {
			traceID = rid
		}
		c.Set("request_id", rid)
		c.Set("trace_id", traceID)
		c.Header("X-Request-ID", rid)
		c.Header(HeaderTraceID, traceID)
		c.Next()
	}
}

func newRequestID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(b)
}