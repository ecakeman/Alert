package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ecakeman/alert/services/platform/apierr"
)

func TestTraceGeneratesAndReusesTraceID(t *testing.T) {
	var gotFromContext string
	handler := Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotFromContext = TraceIDFrom(r.Context())
		w.WriteHeader(http.StatusOK)
	}), Trace(nil))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
	header := rec.Header().Get(HeaderTraceID)
	if !ValidTraceID(header) || header != gotFromContext {
		t.Fatalf("生成 trace 失败: header=%q ctx=%q", header, gotFromContext)
	}

	valid := "0123456789abcdef0123456789abcdef"
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set(HeaderTraceID, valid)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Header().Get(HeaderTraceID) != valid {
		t.Fatalf("合法 trace 未被复用: %q", rec.Header().Get(HeaderTraceID))
	}

	req = httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set(HeaderTraceID, "bad")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Header().Get(HeaderTraceID) == "bad" || !ValidTraceID(rec.Header().Get(HeaderTraceID)) {
		t.Fatalf("非法 trace 未被重新生成: %q", rec.Header().Get(HeaderTraceID))
	}
}

func TestAccessLogContainsTraceAndStatus(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	handler := Chain(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}), Trace(logger), AccessLog(logger))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/services", nil)
	req.Header.Set(HeaderTraceID, "0123456789abcdef0123456789abcdef")
	handler.ServeHTTP(httptest.NewRecorder(), req)

	out := buf.String()
	for _, want := range []string{`"trace_id":"0123456789abcdef0123456789abcdef"`, `"status":201`, `"path":"/api/v1/services"`} {
		if !strings.Contains(out, want) {
			t.Errorf("访问日志缺少 %s，实际 %s", want, out)
		}
	}
}

func TestRecoveryReturnsInternalJSON(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	handler := Chain(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}), Recovery(logger))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"code":"INTERNAL"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestWebClientSuccessAndTracePropagation(t *testing.T) {
	var seenTrace string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenTrace = r.Header.Get(HeaderTraceID)
		_ = json.NewEncoder(w).Encode(map[string]string{"service_id": "s-1"})
	}))
	defer server.Close()

	client := NewClient(server.URL, time.Second)
	ctx := WithTraceID(context.Background(), "0123456789abcdef0123456789abcdef")
	var out map[string]string
	if err := client.GetJSON(ctx, "/internal/services/s-1", &out); err != nil {
		t.Fatalf("GetJSON 失败: %v", err)
	}
	if out["service_id"] != "s-1" {
		t.Fatalf("响应 = %v", out)
	}
	if seenTrace != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("trace 未透传: %q", seenTrace)
	}
}

func TestWebClientUnpacksDomainError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(apierr.Error{Code: apierr.CodeNotFound, Message: "missing"})
	}))
	defer server.Close()

	client := NewClient(server.URL, time.Second)
	err := client.GetJSON(context.Background(), "/internal/services/x", nil)
	var ae *apierr.Error
	if !errors.As(err, &ae) || ae.Code != apierr.CodeNotFound {
		t.Fatalf("错误解包失败: %v", err)
	}
}

func TestWebClientMaps503ToDependencyUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := NewClient(server.URL, time.Second)
	err := client.GetJSON(context.Background(), "/x", nil)
	if apierr.Code(err) != apierr.CodeDependencyUnavailable {
		t.Fatalf("503 映射错误: %v", err)
	}
}

func TestWebClientTimeoutAndSingleAttempt(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL, 50*time.Millisecond)
	err := client.GetJSON(context.Background(), "/x", nil)
	if apierr.Code(err) != apierr.CodeDependencyUnavailable {
		t.Fatalf("超时映射错误: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("请求次数 = %d，期望 1（禁止客户端重试）", got)
	}
}

type stubChecker struct{ err error }

func (s stubChecker) Check(context.Context) error { return s.err }

func TestHealthEndpoints(t *testing.T) {
	health := NewHealthService("monitor", map[string]Checker{
		"db": stubChecker{},
	})
	handler := health.Handler()

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("healthz = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("readyz = %d", rec.Code)
	}

	degraded := NewHealthService("monitor", map[string]Checker{
		"db": stubChecker{err: errors.New("down")},
	})
	rec = httptest.NewRecorder()
	degraded.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("降级 readyz = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"db":"unavailable"`) {
		t.Fatalf("降级响应 = %s", rec.Body.String())
	}
}

func TestHealthcheckExitCodes(t *testing.T) {
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ok.Close()
	if code := Run(ok.URL); code != 0 {
		t.Fatalf("200 → 退出码 %d，期望 0", code)
	}

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer bad.Close()
	if code := Run(bad.URL); code != 1 {
		t.Fatalf("500 → 退出码 %d，期望 1", code)
	}

	if code := Run("http://127.0.0.1:1"); code != 1 {
		t.Fatalf("连接失败 → 退出码 %d，期望 1", code)
	}
}
