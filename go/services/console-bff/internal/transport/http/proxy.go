package bffhttp

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/ecakeman/alert/services/platform/apierr"
	"github.com/ecakeman/alert/services/platform/httpx"
)

type routeSpec struct {
	method   string
	segments []string
	upstream string
	// rewrite 非空时把对外前缀替换为域内前缀（P4.9：/api/v1 → /internal）。
	rewrite string
}

// 18 条 L1 路由的域映射（design 2.2.1）；P1 阶段四个 base URL 均指向单体，P6 起改聚合。
var routes = []routeSpec{
	{http.MethodGet, []string{"api", "v1", "dashboard", "summary"}, "aggregate", ""},
	{http.MethodGet, []string{"api", "v1", "services"}, "registry", "/internal"},
	{http.MethodPost, []string{"api", "v1", "services"}, "registry", "/internal"},
	{http.MethodGet, []string{"api", "v1", "services", "{service_id}"}, "registry", "/internal"},
	{http.MethodPatch, []string{"api", "v1", "services", "{service_id}"}, "registry", "/internal"},
	{http.MethodDelete, []string{"api", "v1", "services", "{service_id}"}, "registry", "/internal"},
	{http.MethodGet, []string{"api", "v1", "services", "{service_id}", "targets"}, "registry", "/internal"},
	{http.MethodPost, []string{"api", "v1", "services", "{service_id}", "targets"}, "registry", "/internal"},
	{http.MethodPatch, []string{"api", "v1", "targets", "{target_id}"}, "registry", "/internal"},
	{http.MethodDelete, []string{"api", "v1", "targets", "{target_id}"}, "registry", "/internal"},
	{http.MethodGet, []string{"api", "v1", "episodes"}, "episode", "/internal"},
	{http.MethodGet, []string{"api", "v1", "episodes", "{episode_id}"}, "episode", "/internal"},
	{http.MethodPost, []string{"api", "v1", "services", "{service_id}", "events"}, "episode", "/internal"},
	{http.MethodGet, []string{"api", "v1", "services", "{service_id}", "timeline"}, "aggregate", ""},
	{http.MethodGet, []string{"api", "v1", "episodes", "{episode_id}", "timeline"}, "aggregate", ""},
	{http.MethodGet, []string{"api", "v1", "episodes", "{episode_id}", "artifacts"}, "task", "/internal"},
	{http.MethodPost, []string{"api", "v1", "episodes", "{episode_id}", "retry-diagnosis"}, "task", "/internal"},
	{http.MethodPost, []string{"api", "v1", "episodes", "{episode_id}", "retry-archive"}, "task", "/internal"},
}

// ProxyConfig 是 BFF 反代的四个域地址与上游超时。
type ProxyConfig struct {
	Registry  string
	Monitor   string
	Episode   string
	Task      string
	Aggregate http.Handler
	Timeout   time.Duration
	Logger    *slog.Logger
}

// Proxy 按白名单将 L1 路由透明反代到目标域。
type Proxy struct {
	routes  []routeSpec
	proxies map[string]http.Handler
	logger  *slog.Logger
}

// NewProxy 为四个域各建一个 ReverseProxy；地址非法 patch 为配置错误。
func NewProxy(cfg ProxyConfig) (*Proxy, error) {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 3 * time.Second
	}
	proxies := map[string]http.Handler{"aggregate": cfg.Aggregate}
	for name, base := range map[string]string{
		"registry": cfg.Registry,
		"monitor":  cfg.Monitor,
		"episode":  cfg.Episode,
		"task":     cfg.Task,
	} {
		handler, err := newReverseProxy(base, cfg.Timeout)
		if err != nil {
			return nil, fmt.Errorf("bff upstream %s: %w", name, err)
		}
		proxies[name] = handler
	}
	return &Proxy{routes: routes, proxies: proxies, logger: cfg.Logger}, nil
}

func newReverseProxy(base string, timeout time.Duration) (http.Handler, error) {
	target, err := url.Parse(base)
	if err != nil || (target.Scheme != "http" && target.Scheme != "https") || target.Host == "" {
		return nil, fmt.Errorf("invalid base url")
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Transport = &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   timeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ResponseHeaderTimeout: timeout,
		MaxIdleConnsPerHost:   64,
		IdleConnTimeout:       30 * time.Second,
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		apierr.Write(w, apierr.DependencyUnavailable("upstream unavailable"))
	}
	return proxy, nil
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	segments := pathSegments(r.URL.Path)
	var allowed []string
	for _, route := range p.routes {
		if !matchPath(route.segments, segments) {
			continue
		}
		if route.method != r.Method {
			allowed = append(allowed, route.method)
			continue
		}
		upstream := p.proxies[route.upstream]
		if upstream == nil {
			apierr.Write(w, apierr.DependencyUnavailable("upstream not configured"))
			return
		}
		if traceID := httpx.TraceIDFrom(r.Context()); traceID != "" {
			r.Header.Set(httpx.HeaderTraceID, traceID)
		}
		if route.rewrite != "" {
			r = rewritePath(r, route.rewrite)
		}
		upstream.ServeHTTP(w, r)
		return
	}
	if len(allowed) > 0 {
		w.Header().Set("Allow", joinUnique(allowed))
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	notFound(w, r)
}

// rewritePath 把 /api/v1/** 重写为 <prefix>/**，保留查询串（克隆请求，避免污染原对象）。
func rewritePath(r *http.Request, prefix string) *http.Request {
	clone := r.Clone(r.Context())
	trimmed := strings.TrimPrefix(clone.URL.Path, "/api/v1")
	clone.URL.Path = prefix + trimmed
	clone.URL.RawPath = ""
	return clone
}

func pathSegments(path string) []string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}

func matchPath(pattern, actual []string) bool {
	if len(pattern) != len(actual) {
		return false
	}
	for i, want := range pattern {
		if strings.HasPrefix(want, "{") && strings.HasSuffix(want, "}") {
			if actual[i] == "" {
				return false
			}
			continue
		}
		if want != actual[i] {
			return false
		}
	}
	return true
}

func joinUnique(values []string) string {
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return strings.Join(out, ", ")
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"code": code, "message": message})
}
