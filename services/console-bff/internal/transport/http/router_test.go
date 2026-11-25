package bffhttp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ecakeman/alert/services/platform/httpx"
)

type stubChecker struct{ err error }

func (s stubChecker) Check(context.Context) error { return s.err }

func TestRouterHealthAndReady(t *testing.T) {
	health := httpx.NewHealthService("console-bff", map[string]httpx.Checker{"registry": stubChecker{}})
	handler := New(Deps{Health: health})

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
}

func TestRouterReadyDegraded(t *testing.T) {
	health := httpx.NewHealthService("console-bff", map[string]httpx.Checker{
		"registry": stubChecker{err: errors.New("down")},
	})
	handler := New(Deps{Health: health})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("降级 readyz = %d，期望 503", rec.Code)
	}
}

func TestRouterUnknownPathIsJSON404(t *testing.T) {
	health := httpx.NewHealthService("console-bff", nil)
	handler := New(Deps{Health: health})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/unknown", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"code":"NOT_FOUND"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}