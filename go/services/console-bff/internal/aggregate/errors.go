package aggregate

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/ecakeman/alert/services/platform/apierr"
)

// writeNestedError 输出与 P0 一致的嵌套错误信封。
func writeNestedError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{"code": code, "message": message, "request_id": newRequestID()},
	})
}

func newRequestID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// isUpstream404 判断上游域服务是否返回 404（httpx 构造 "upstream status 404"）。
func isUpstream404(err error) bool {
	if err == nil {
		return false
	}
	if apierr.Code(err) == apierr.CodeNotFound {
		return true
	}
	return strings.Contains(err.Error(), "upstream status 404")
}