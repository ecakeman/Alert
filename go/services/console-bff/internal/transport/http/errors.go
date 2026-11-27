package bffhttp

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// apiError 输出与 P0 一致的嵌套错误信封：{"error":{code,message,request_id}}。
func apiError(w http.ResponseWriter, status int, code, message string) {
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

// normalizeUpstreamError 把上游域服务返回的扁平 {code,message} 改写为嵌套信封（保持状态码）。
// 已是嵌套（含 error 键）或非 JSON 则原样返回。
func normalizeUpstreamError(resp *http.Response) error {
	if resp.Body == nil || !strings.Contains(resp.Header.Get("Content-Type"), "application/json") {
		return nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxL1BodyBytes+1))
	_ = resp.Body.Close()
	if err != nil {
		return err
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(body, &m) == nil {
		if _, nested := m["error"]; !nested {
			if rawCode, ok := m["code"]; ok {
				var code, message string
				_ = json.Unmarshal(rawCode, &code)
				if rawMsg, ok2 := m["message"]; ok2 {
					_ = json.Unmarshal(rawMsg, &message)
				}
				out, _ := json.Marshal(map[string]any{
					"error": map[string]string{"code": code, "message": message, "request_id": newRequestID()},
				})
				resp.Body = io.NopCloser(bytes.NewReader(out))
				resp.ContentLength = int64(len(out))
				resp.Header.Set("Content-Length", strconv.Itoa(len(out)))
				resp.Header.Del("Transfer-Encoding")
				return nil
			}
		}
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))
	return nil
}