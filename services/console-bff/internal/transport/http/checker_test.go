package bffhttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ecakeman/alert/services/platform/httpx"
)

func TestDownstreamCheckerCachesResult(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer server.Close()

	checker := &DownstreamChecker{
		Name:     "registry",
		Client:   httpx.NewClient(server.URL, time.Second),
		CacheTTL: time.Minute,
	}
	for i := 0; i < 3; i++ {
		if err := checker.Check(context.Background()); err != nil {
			t.Fatalf("Check 失败: %v", err)
		}
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("缓存期内请求次数 = %d，期望 1", got)
	}
}

func TestDownstreamCheckerReportsFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	checker := &DownstreamChecker{Client: httpx.NewClient(server.URL, time.Second), CacheTTL: time.Minute}
	if err := checker.Check(context.Background()); err == nil {
		t.Fatal("下游 503 应返回错误")
	}
}