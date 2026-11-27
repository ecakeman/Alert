package bffhttp

import (

	"log/slog"
	"net/http"
	"time"

	"github.com/ecakeman/alert/services/platform/httpx"
)

// Deps 是 BFF 路由装配依赖；P1.1 只有探针，P1.2 接入 18 路由反代。
type Deps struct {
	Health  *httpx.HealthService
	Logger  *slog.Logger
	Proxy   http.Handler
	Timeout time.Duration
}

// New 返回 BFF 的根 handler：探针 +（P1.2 起）白名单反代 + 统一 JSON 404。
func New(deps Deps) http.Handler {
	mux := http.NewServeMux()
	health := deps.Health.Handler()
	mux.Handle("/healthz", health)
	mux.Handle("/readyz", health)
	if deps.Proxy != nil {
		mux.Handle("/api/v1/", deps.Proxy)
	}
	mux.HandleFunc("/", notFound)
	return mux
}

func notFound(w http.ResponseWriter, _ *http.Request) {
	apiError(w, http.StatusNotFound, "NOT_FOUND", "resource not found")
}
