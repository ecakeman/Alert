package bffhttp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/ecakeman/alert/services/console-bff/internal/aggregate"
)

type fakeProjector struct {
	mu    sync.Mutex
	calls int
	last  aggregate.L1Route
}

func (f *fakeProjector) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusTeapot)
}

func (f *fakeProjector) ProjectL1(_ context.Context, route aggregate.L1Route, upstream []byte) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.last = route
	return upstream, nil
}

func (f *fakeProjector) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func upstreamServer(t *testing.T, name string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Upstream", name)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"upstream": name,
			"method":   r.Method,
			"path":     r.URL.Path,
			"body":     string(body),
		})
	}))
}

func newProxyForTest(t *testing.T) (*Proxy, *fakeProjector) {
	t.Helper()
	servers := map[string]*httptest.Server{
		"registry": upstreamServer(t, "registry"),
		"monitor":  upstreamServer(t, "monitor"),
		"episode":  upstreamServer(t, "episode"),
		"task":     upstreamServer(t, "task"),
	}
	t.Cleanup(func() {
		for _, server := range servers {
			server.Close()
		}
	})
	projector := &fakeProjector{}
	proxy, err := NewProxy(ProxyConfig{
		Registry:  servers["registry"].URL,
		Monitor:   servers["monitor"].URL,
		Episode:   servers["episode"].URL,
		Task:      servers["task"].URL,
		Aggregate: projector,
		Timeout:   time.Second,
	})
	if err != nil {
		t.Fatalf("NewProxy 失败: %v", err)
	}
	return proxy, projector
}

func findRoute(t *testing.T, method, path string) aggregate.L1Route {
	t.Helper()
	segments := pathSegments(path)
	for _, route := range routes {
		if route.method == method && matchPath(route.segments, segments) {
			return route.l1
		}
	}
	t.Fatalf("未找到路由 %s %s", method, path)
	return aggregate.L1None
}

func TestRouteL1Mapping(t *testing.T) {
	cases := []struct {
		method string
		path   string
		want   aggregate.L1Route
	}{
		{http.MethodGet, "/api/v1/dashboard/summary", aggregate.L1None},
		{http.MethodPost, "/api/v1/services", aggregate.L1ServiceViewNoActive},
		{http.MethodGet, "/api/v1/services/svc-1", aggregate.L1None},
		{http.MethodPatch, "/api/v1/services/svc-1", aggregate.L1ServiceViewActive},
		{http.MethodDelete, "/api/v1/services/svc-1", aggregate.L1None},
		{http.MethodGet, "/api/v1/services/svc-1/targets", aggregate.L1TargetsList},
		{http.MethodPost, "/api/v1/services/svc-1/targets", aggregate.L1Target},
		{http.MethodPatch, "/api/v1/targets/tgt-1", aggregate.L1Target},
		{http.MethodDelete, "/api/v1/targets/tgt-1", aggregate.L1None},
	}
	for _, tc := range cases {
		if got := findRoute(t, tc.method, tc.path); got != tc.want {
			t.Fatalf("%s %s l1 = %v，期望 %v", tc.method, tc.path, got, tc.want)
		}
	}
}

func TestRegistryDirectRoutes(t *testing.T) {
	proxy, _ := newProxyForTest(t)
	cases := []struct {
		method string
		path   string
		status int
	}{
		{http.MethodPost, "/api/v1/services", 200},
		{http.MethodPatch, "/api/v1/services/svc-1", 200},
		{http.MethodDelete, "/api/v1/services/svc-1", 204},
		{http.MethodGet, "/api/v1/services/svc-1/targets", 200},
		{http.MethodPost, "/api/v1/services/svc-1/targets", 200},
		{http.MethodPatch, "/api/v1/targets/tgt-1", 200},
		{http.MethodDelete, "/api/v1/targets/tgt-1", 204},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			proxy.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))
			if rec.Code != tc.status {
				t.Fatalf("status = %d，期望 %d", rec.Code, tc.status)
			}
			if tc.status == http.StatusNoContent {
				return
			}
			var body map[string]string
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Fatalf("解析响应失败: %v", err)
			}
			if body["upstream"] != "registry" {
				t.Fatalf("upstream = %s，期望 registry", body["upstream"])
			}
		})
	}
}

func TestPatchServiceProjectedOnce(t *testing.T) {
	proxy, projector := newProxyForTest(t)
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, httptest.NewRequest(http.MethodPatch, "/api/v1/services/svc-1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if projector.count() != 1 {
		t.Fatalf("投影次数 = %d，期望 1", projector.count())
	}
	if projector.last != aggregate.L1ServiceViewActive {
		t.Fatalf("last route = %v，期望 L1ServiceViewActive", projector.last)
	}
}

func TestDeleteNotProjected(t *testing.T) {
	proxy, projector := newProxyForTest(t)
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/v1/services/svc-1", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d，期望 204", rec.Code)
	}
	if projector.count() != 0 {
		t.Fatalf("204 不应触发投影，实际 %d 次", projector.count())
	}
}

func TestNewProxyFailsWithoutProjector(t *testing.T) {
	_, err := NewProxy(ProxyConfig{
		Registry:  "http://127.0.0.1:1",
		Monitor:   "http://127.0.0.1:1",
		Episode:   "http://127.0.0.1:1",
		Task:      "http://127.0.0.1:1",
		Aggregate: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
		Timeout:   time.Second,
	})
	if err == nil {
		t.Fatal("缺少 L1Projector 时应构造失败")
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

	projector := &fakeProjector{}
	proxy, err := NewProxy(ProxyConfig{
		Registry: slow.URL, Monitor: slow.URL, Episode: slow.URL, Task: slow.URL,
		Aggregate: projector, Timeout: 30 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewProxy 失败: %v", err)
	}
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/services/svc-1/targets", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("超时 = %d，期望 503", rec.Code)
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&body)
	if body.Error.Code != "DEPENDENCY_UNAVAILABLE" {
		t.Fatalf("503 code = %s，期望 DEPENDENCY_UNAVAILABLE", body.Error.Code)
	}
}
