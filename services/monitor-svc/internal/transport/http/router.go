package monitorhttp

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/ecakeman/alert/services/platform/httpx"
)

// Deps 是 monitor 路由装配依赖；P3.1 只有探针，P3.5 接入域 API。
type Deps struct {
	Health *httpx.HealthService
	Logger *slog.Logger
	API    http.Handler
}

// New 返回 monitor 的根 handler：探针 +（P3.5 起）域 API + 统一 JSON 404。
func New(deps Deps) http.Handler {
	mux := http.NewServeMux()
	health := deps.Health.Handler()
	mux.Handle("/healthz", health)
	mux.Handle("/readyz", health)
	if deps.API != nil {
		mux.Handle("/internal/", deps.API)
	}
	mux.HandleFunc("/", notFound)
	return mux
}

func notFound(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"code":    "NOT_FOUND",
		"message": "resource not found",
	})
}