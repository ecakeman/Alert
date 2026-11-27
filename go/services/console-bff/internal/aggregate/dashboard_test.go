package aggregate

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ecakeman/alert/services/platform/httpx"
)

func jsonHandler(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}
}

func newClientsForTest(t *testing.T) Clients {
	t.Helper()
	trend := make([]MonitorTrendPoint, 24)
	for i := range trend {
		trend[i] = MonitorTrendPoint{BucketStart: fmt.Sprintf("2026-11-01T%02d:00:00Z", i), Success: 0, Failure: 0}
	}
	trendJSON, _ := json.Marshal(trend)

	registryMux := http.NewServeMux()
	registryMux.HandleFunc("/internal/stats", jsonHandler(`{"services":{"total":3,"enabled":2,"disabled":1,"normal":1,"abnormal":1,"unknown":1},"targets":{"NORMAL":2,"ABNORMAL":1}}`))
	registryMux.HandleFunc("/internal/services", jsonHandler(`{"services":[{"service_id":"s1","name":"Checkout API"}]}`))
	monitorMux := http.NewServeMux()
	monitorMux.HandleFunc("/internal/stats", jsonHandler(`{"services":{},"targets":{}}`))
	monitorMux.HandleFunc("/internal/points/trend", jsonHandler(string(trendJSON)))
	episodeMux := http.NewServeMux()
	episodeMux.HandleFunc("/internal/stats", jsonHandler(`{"active":1,"archived":0,"resolved":0}`))
	episodeMux.HandleFunc("/internal/episodes", jsonHandler(`{"total":1,"episodes":[{"episode_id":"e1","service_id":"s1","status":"ACTIVE","started_by":"MONITOR","resolution_pending":false,"input_version":2,"processed_input_version":1,"started_at":"2026-11-01T00:00:00Z","updated_at":"2026-11-01T00:00:00Z","resolved_at":null,"archived_at":null}]}`))
	taskMux := http.NewServeMux()
	taskMux.HandleFunc("/internal/stats", jsonHandler(`{"statuses":{"RUNNING":1}}`))
	taskMux.HandleFunc("/internal/batch/active-tasks", jsonHandler(`{"e1":{"task_id":"t1","task_type":"DIAGNOSIS","status":"RUNNING","generation":1}}`))
	taskMux.HandleFunc("/internal/batch/report-flags", jsonHandler(`{"e1":true}`))

	servers := []*httptest.Server{
		httptest.NewServer(registryMux), httptest.NewServer(monitorMux),
		httptest.NewServer(episodeMux), httptest.NewServer(taskMux),
	}
	t.Cleanup(func() {
		for _, s := range servers {
			s.Close()
		}
	})
	return Clients{
		Registry: httpx.NewClient(servers[0].URL, time.Second),
		Monitor:  httpx.NewClient(servers[1].URL, time.Second),
		Episode:  httpx.NewClient(servers[2].URL, time.Second),
		Task:     httpx.NewClient(servers[3].URL, time.Second),
	}
}

func TestDashboardShapeAndEnrichment(t *testing.T) {
	svc := New(newClientsForTest(t), 2*time.Second, nil)
	rec := httptest.NewRecorder()
	svc.dashboard(rec, httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/summary", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out Dashboard
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.MonitorTrend) != 24 {
		t.Fatalf("monitor_trend=%d，期望 24", len(out.MonitorTrend))
	}
	if out.Services.Total != 3 || out.Services.Normal != 1 || out.Services.Abnormal != 1 || out.Services.Unknown != 1 {
		t.Fatalf("services 分布不正确: %+v", out.Services)
	}
	if out.Targets.Total != 3 || out.Targets.Normal != 2 || out.Targets.Abnormal != 1 {
		t.Fatalf("targets 分布不正确: %+v", out.Targets)
	}
	if len(out.RecentEpisodes) != 1 {
		t.Fatalf("recent=%d", len(out.RecentEpisodes))
	}
	r := out.RecentEpisodes[0]
	if r.ServiceName != "Checkout API" || r.ActiveTaskType != "DIAGNOSIS" || r.ActiveTaskStatus != "RUNNING" || !r.HasDiagnosisReport {
		t.Fatalf("recent 补齐不正确: %+v", r)
	}
}

func TestDashboardUpstreamError(t *testing.T) {
	clients := newClientsForTest(t)
	clients.Episode = httpx.NewClient("http://127.0.0.1:1", 100*time.Millisecond)
	svc := New(clients, 500*time.Millisecond, nil)
	rec := httptest.NewRecorder()
	svc.dashboard(rec, httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/summary", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("上游失败应 503，实际 %d", rec.Code)
	}
}