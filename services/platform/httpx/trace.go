package httpx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"regexp"
)

// HeaderTraceID 与 contracts.HeaderTraceID 保持同名字面值；platform 不 import contracts。
const HeaderTraceID = "X-Trace-Id"

// TraceIDLength 是追踪 ID 的固定长度（32 个小写十六进制字符）。
const TraceIDLength = 32

var traceIDRe = regexp.MustCompile(`^[0-9a-f]{32}$`)

type traceKey struct{}

// NewTraceID 生成 32 hex 追踪 ID；随机源失败返回空串。
func NewTraceID() string {
	var buf [TraceIDLength / 2]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return ""
	}
	return hex.EncodeToString(buf[:])
}

// WithTraceID 把追踪 ID 放入 context。
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceKey{}, traceID)
}

// TraceIDFrom 从 context 读取追踪 ID；不存在返回空串。
func TraceIDFrom(ctx context.Context) string {
	value, _ := ctx.Value(traceKey{}).(string)
	return value
}

// ValidTraceID 判断是否为 32 位小写十六进制。
func ValidTraceID(id string) bool { return traceIDRe.MatchString(id) }
