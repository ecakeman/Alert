package httpx

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// Checker 是依赖健康检查（如 db.Pool.Check）。
type Checker interface {
	Check(ctx context.Context) error
}

// HealthService 提供 /healthz（进程存活）与 /readyz（依赖就绪）。
type HealthService struct {
	Name         string
	Checkers     map[string]Checker
	CheckTimeout time.Duration
}

// NewHealthService 构建健康探针。
func NewHealthService(name string, checkers map[string]Checker) *HealthService {
	return &HealthService{Name: name, Checkers: checkers, CheckTimeout: 2 * time.Second}
}

// Handler 返回健康探针路由。
func (h *HealthService) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": h.Name})
	})
	mux.HandleFunc("/readyz", h.readyz)
	return mux
}

func (h *HealthService) readyz(w http.ResponseWriter, r *http.Request) {
	checks := make(map[string]string, len(h.Checkers))
	ready := true
	var mu sync.Mutex
	var wg sync.WaitGroup
	for name, checker := range h.Checkers {
		wg.Add(1)
		go func(name string, checker Checker) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(r.Context(), h.CheckTimeout)
			defer cancel()
			err := checker.Check(ctx)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				ready = false
				checks[name] = "unavailable"
				return
			}
			checks[name] = "ok"
		}(name, checker)
	}
	wg.Wait()

	status := http.StatusOK
	state := "ok"
	if !ready {
		status = http.StatusServiceUnavailable
		state = "degraded"
	}
	writeJSON(w, status, map[string]any{"status": state, "service": h.Name, "checks": checks})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
