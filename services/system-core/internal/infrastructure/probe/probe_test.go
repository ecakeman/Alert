package probe

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	portService "github.com/ecakeman/alert/services/system-core/internal/application/port/service"
)

func TestHTTPProbe(t *testing.T) {
	okSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer okSrv.Close()
	badSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusInternalServerError) }))
	defer badSrv.Close()

	p := NewHTTPProber()
	if s := p.Probe(context.Background(), portService.MonitorTarget{Address: okSrv.URL, TimeoutMS: 2000}); s.CheckStatus != "SUCCESS" || s.StatusClass != "2xx" {
		t.Fatalf("expected 2xx success, got %+v", s)
	}
	if s := p.Probe(context.Background(), portService.MonitorTarget{Address: badSrv.URL, TimeoutMS: 2000}); s.CheckStatus != "FAILURE" || s.StatusClass != "5xx" {
		t.Fatalf("expected 5xx failure, got %+v", s)
	}
}

func TestTCPProbe(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()

	p := NewTCPProber()
	if s := p.Probe(context.Background(), portService.MonitorTarget{Address: addr, TimeoutMS: 1000}); s.CheckStatus != "SUCCESS" {
		t.Fatalf("expected tcp success, got %+v", s)
	}
	_ = ln.Close()
	if s := p.Probe(context.Background(), portService.MonitorTarget{Address: "127.0.0.1:1", TimeoutMS: 300}); s.CheckStatus == "SUCCESS" {
		t.Fatalf("closed port must not succeed, got %+v", s)
	}
}
