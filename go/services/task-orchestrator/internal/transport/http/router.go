package taskhttp

import (
	"encoding/json"
	"net/http"

	"github.com/ecakeman/alert/services/platform/httpx"
)

// Deps 是 task 路由装配依赖；P5.1 只有探针，P5.6 接入内部 API。
type Deps struct {
	Health *httpx.HealthService
	API    http.Handler
}

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