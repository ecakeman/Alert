package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ecakeman/alert/internal/application/health"
	"github.com/ecakeman/alert/internal/transport/http/handler"
	"github.com/ecakeman/alert/internal/transport/http/middleware"
)

func newTestEngine() *gin.Engine {
	return New(Deps{Health: handler.NewHealth(health.NewService()), Timeout: 5 * time.Second})
}

func concretePath(path string) string {
	path = strings.ReplaceAll(path, ":service_id", "s")
	path = strings.ReplaceAll(path, ":episode_id", "e")
	path = strings.ReplaceAll(path, ":target_id", "t")
	return "/api/v1" + path
}

// 未施工路由（pendingRoutes）保持 501。
func TestPendingRoutes501(t *testing.T) {
	r := newTestEngine()
	for _, rt := range pendingRoutes {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(rt.method, concretePath(rt.path), nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusNotImplemented {
			t.Fatalf("%s %s expected 501, got %d", rt.method, rt.path, w.Code)
		}
	}
}

func TestNotImplementedAndRequestID(t *testing.T) {
	r := newTestEngine()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/services/s/timeline", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d", w.Code)
	}
	if w.Header().Get("X-Request-ID") == "" {
		t.Fatal("missing X-Request-ID header")
	}
	if !strings.Contains(w.Body.String(), "NOT_IMPLEMENTED") {
		t.Fatalf("expected NOT_IMPLEMENTED body, got %s", w.Body.String())
	}
}

func TestNoRoute404(t *testing.T) {
	r := newTestEngine()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/not-a-route", nil))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "NOT_FOUND") {
		t.Fatalf("expected NOT_FOUND body, got %s", w.Body.String())
	}
}

func TestHealthz(t *testing.T) {
	r := newTestEngine()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestRecoveryPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.Recovery())
	r.GET("/boom", func(c *gin.Context) { panic("boom") })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/boom", nil))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "INTERNAL") {
		t.Fatalf("expected INTERNAL body, got %s", w.Body.String())
	}
}
