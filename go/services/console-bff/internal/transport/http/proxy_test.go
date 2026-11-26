package bffhttp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ecakeman/alert/services/platform/httpx"
)

func upstream(t *testing.T, name string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("X-Upstream", name)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"upstream": name,
			"method":   r.Method,
			"path":     r.URL.Path,
			"query":    r.URL.RawQuery,
			"trace":    r.Header.Get(httpx.HeaderTraceID),
			"body":     string(body),
		})
	}))
}

func newProxyForTest(t *testing.T) (*Proxy, map[string]*httptest.Server) {
	t.Helper()
	servers := map[string]*httptest.Server{
		"registry": upstream(t, "registry"),
		"monitor":  upstream(t, "monitor"),
		"episode":  upstream(t, "episode"),
		"task":     upstream(t, "task"),
	}
	t.Cleanup(func() {
		for _, server := range servers {
			server.Close()
		}
	})
	proxy, err := NewProxy(ProxyConfig{
		Registry: servers["registry"].URL,
		Monitor:  servers["monitor"].URL,
		Episode:  servers["episode"].URL,
		Task:     servers["task"].URL,
		Timeout:  time.Second,
	})
	if err != nil {
		t.Fatalf("NewProxy 失败: %v", err)
	}
	return proxy, servers
}

func TestProxyRoutesAll18ToExpectedUpstream(t *testing.T) {
	proxy, _ := newProxyForTest(t)
	cases := []struct {
		method string
		path   string
		want   string
	}{
		{http.MethodGet, "/api/v1/dashboard/summary", "episode"},
		{http.MethodGet, "/api/v1/services", "registry"},
		{http.MethodPost, "/api/v1/services", "registry"},
		{http.MethodGet, "/api/v1/services/svc-1", "registry"},
		{http.MethodPatch, "/api/v1/services/svc-1", "registry"},
		{http.MethodDelete, "/api/v1/services/svc-1", "registry"},
		{http.MethodGet, "/api/v1/services/svc-1/targets", "registry"},
		{http.MethodPost, "/api/v1/services/svc-1/targets", "registry"},
		{http.MethodPatch, "/api/v1/targets/tgt-1", "registry"},
		{http.MethodDelete, "/api/v1/targets/tgt-1", "registry"},
		{http.MethodGet, "/api/v1/episodes", "episode"},
		{http.MethodGet, "/api/v1/episodes/ep-1", "episode"},
		{http.MethodPost, "/api/v1/services/svc-1/events", "episode"},
		{http.MethodGet, "/api/v1/services/svc-1/timeline", "episode"},
		{http.MethodGet, "/api/v1/episodes/ep-1/timeline", "episode"},
		{http.MethodGet, "/api/v1/episodes/ep-1/artifacts", "task"},
		{http.MethodPost, "/api/v1/episodes/ep-1/retry-diagnosis", "task"},
		{http.MethodPost, "/api/v1/episodes/ep-1/retry-archive", "task"},
	}
	if len(cases) != 18 {
		t.Fatalf("路由数 = %d，期望 18", len(cases))
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, tc.path+"?limit=10&offset=0", nil)
			proxy.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d", rec.Code)
			}
			var body map[string]string
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Fatalf("解析响应失败: %v", err)
			}
			if body["upstream"] != tc.want {
				t.Fatalf("upstream = %s，期望 %s", body["upstream"], tc.want)
			}
			if body["path"] != tc.path {
				t.Fatalf("path 未原样透传: %s", body["path"])
			}
			if body["query"] != "limit=10&offset=0" {
				t.Fatalf("query 未原样透传: %s", body["query"])
			}
		})
	}
}

func TestProxyForwardsBodyAndTrace(t *testing.T) {
	proxy, _ := newProxyForTest(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/services", stringsReader(`{"service_key":"x"}`))
	req = req.WithContext(httpx.WithTraceID(context.Background(), "0123456789abcdef0123456789abcdef"))
	proxy.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if rec.Header().Get("X-Upstream") != "registry" {
		t.Fatalf("响应头未透传: %v", rec.Header())
	}
	var body map[string]string
	_ = json.NewDecoder(rec.Body).Decode(&body)
	if body["trace"] != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("trace 未透传: %s", body["trace"])
	}
	if body["body"] != `{"service_key":"x"}` {
		t.Fatalf("请求体未透传: %s", body["body"])
	}
}

func TestProxyUnknownPathAndWrongMethod(t *testing.T) {
	proxy, _ := newProxyForTest(t)

	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/unknown", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("未知路径 = %d，期望 404", rec.Code)
	}

	rec = httptest.NewRecorder()
	proxy.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/v1/services/svc-1", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("错误方法 = %d，期望 405", rec.Code)
	}
	if rec.Header().Get("Allow") == "" {
		t.Fatal("405 缺少 Allow 头")
	}
}

func TestProxyUpstreamTimeoutMapsTo503(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(150 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer slow.Close()

	proxy, err := NewProxy(ProxyConfig{
		Registry: slow.URL, Monitor: slow.URL, Episode: slow.URL, Task: slow.URL,
		Timeout: 30 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewProxy 失败: %v", err)
	}
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/services", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("超时 = %d，期望 503", rec.Code)
	}
}

func stringsReader(s string) io.Reader { return strings.NewReader(s) }
