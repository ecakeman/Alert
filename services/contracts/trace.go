package contracts

import (
	"crypto/rand"
	"encoding/hex"
)

// HeaderTraceID 是 BFF → 域服务 → 事件信封共用的追踪头。
const HeaderTraceID = "X-Trace-Id"

// TraceIDLength 是追踪 ID 的固定长度（32 个小写十六进制字符）。
const TraceIDLength = 32

// NewTraceID 生成 32 hex 的追踪 ID；随机源失败时返回空串，由调用方决定是否降级为日志标记。
func NewTraceID() string {
	var buf [TraceIDLength / 2]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return ""
	}
	return hex.EncodeToString(buf[:])
}
