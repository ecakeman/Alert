package bffhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ecakeman/alert/services/console-bff/internal/aggregate"
	"github.com/ecakeman/alert/services/platform/apierr"
	"github.com/ecakeman/alert/services/platform/httpx"
)

const maxL1BodyBytes = 8 << 20

type l1ContextKey struct{}

type routeSpec struct {
	method   string
	segments []string
	upstream string
	// rewrite 非空时把对外前缀替换为域内前缀（P4.9：/api/v1 → /internal）。
	rewrite string
	// l1 标记该路由在 L1 出口需要的投影类型（D-24，L1None 表示不投影）。
	l1 aggregate.L1Route
}

// 18 条 L1 路由的域映射（design 2.2.1）；P6 起目录域直连 registry，dashboard/时间轴走 aggregate。
var routes = []routeSpec{
	{http.MethodGet, []string{"api", "v1", "dashboard", "summary"}, "aggregate", "", aggregate.L1None},
	{http.MethodGet, []string{"api", "v1", "services"}, "aggregate", "", aggregate.L1None},
	{http.MethodPost, []string{"api", "v1", "services"}, "registry", "/internal", aggregate.L1ServiceViewNoActive},
	{http.MethodGet, []string{"api", "v1", "services", "{service_id}"}, "aggregate", "", aggregate.L1None},
	{http.MethodPatch, []string{"api", "v1", "services", "{service_id}"}, "registry", "/internal", aggregate.L1ServiceViewActive},
	{http.MethodDelete, []string{"api", "v1", "services", "{service_id}"}, "registry", "/internal", aggregate.L1None},
	{http.MethodGet, []string{"api", "v1", "services", "{service_id}", "targets"}, "registry", "/internal", aggregate.L1TargetsList},
	{http.MethodPost, []string{"api", "v1", "services", "{service_id}", "targets"}, "registry", "/internal", aggregate.L1Target},
	{http.MethodPatch, []string{"api", "v1", "targets", "{target_id}"}, "registry", "/internal", aggregate.L1Target},
	{http.MethodDelete, []string{"api", "v1", "targets", "{target_id}"}, "registry", "/internal", aggregate.L1None},
	{http.MethodGet, []string{"api", "v1", "episodes"}, "aggregate", "", aggregate.L1None},
	{http.MethodGet, []string{"api", "v1", "episodes", "{episode_id}"}, "aggregate", "", aggregate.L1None},
	{http.MethodPost, []string{"api", "v1", "services", "{service_id}", "events"}, "episode", "/internal", aggregate.L1None},
	{http.MethodGet, []string{"api", "v1", "services", "{service_id}", "timeline"}, "aggregate", "", aggregate.L1None},
	{http.MethodGet, []string{"api", "v1", "episodes", "{episode_id}", "timeline"}, "aggregate", "", aggregate.L1None},
	{http.MethodGet, []string{"api", "v1", "episodes", "{episode_id}", "artifacts"}, "task", "/internal", aggregate.L1None},
	{http.MethodPost, []string{"api", "v1", "episodes", "{episode_id}", "retry-diagnosis"}, "task", "/internal", aggregate.L1None},
	{http.MethodPost, []string{"api", "v1", "episodes", "{episode_id}", "retry-archive"}, "task", "/internal", aggregate.L1None},
}

// ProxyConfig 是 BFF 反代的四个域地址、聚合实现与上游超时。
type ProxyConfig struct {
	Registry  string
	Monitor   string
	Episode   string
	Task      string
	Aggregate http.Handler
	Timeout   time.Duration
	Logger    *slog.Logger
}

// Proxy 按白名单将 L1 路由透明反代到目标域，并在 L1 出口做契约投影。
type Proxy struct {
	routes  []routeSpec
	proxies map[string]http.Handler
	logger  *slog.Logger
}

// NewProxy 为四个域各建一个 ReverseProxy；存在投影路由时必须注入 L1Projector（fail-fast）。
func NewProxy(cfg ProxyConfig) (*Proxy, error) {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 3 * time.Second
	}

	projector, _ := cfg.Aggregate.(aggregate.L1Projector)
	needsProjection := false
	for _, rt := range routes {
		if rt.l1 != aggregate.L1None {
			needsProjection = true
			break
		}
	}
	if needsProjection && projector == nil {
		return nil, fmt.Errorf("bff aggregate does not implement L1Projector while routes require projection")
	}

	proxies := map[string]http.Handler{"aggregate": cfg.Aggregate}
	for name, base := range map[string]string{
		"registry": cfg.Registry,
		"monitor":  cfg.Monitor,
		"episode":  cfg.Episode,
		"task":     cfg.Task,
	} {
		handler, err := newReverseProxy(base, cfg.Timeout, projector)
		if err != nil {
			return nil, fmt.Errorf("bff upstream %s: %w", name, err)
		}
		proxies[name] = handler
	}
	return &Proxy{routes: routes, proxies: proxies, logger: cfg.Logger}, nil
}

func newReverseProxy(base string, timeout time.Duration, projector aggregate.L1Projector) (http.Handler, error) {
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
	proxy.ModifyResponse = func(resp *http.Response) error {
		route, ok := resp.Request.Context().Value(l1ContextKey{}).(aggregate.L1Route)
		if !ok || route == aggregate.L1None {
			return nil
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil
		}
		if !strings.Contains(resp.Header.Get("Content-Type"), "application/json") {
			return fmt.Errorf("l1 projection: unexpected content-type %q", resp.Header.Get("Content-Type"))
		}
		if resp.Body == nil {
			return nil
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxL1BodyBytes+1))
		if closeErr := resp.Body.Close(); closeErr != nil && readErr == nil {
			readErr = closeErr
		}
		if readErr != nil {
			return fmt.Errorf("l1 projection: read body: %w", readErr)
		}
		if len(body) > maxL1BodyBytes {
			return fmt.Errorf("l1 projection: body exceeds %d bytes", maxL1BodyBytes)
		}
		if len(body) == 0 {
			return nil
		}
		if projector == nil {
			return fmt.Errorf("l1 projection: projector missing")
		}
		out, err := projector.ProjectL1(resp.Request.Context(), route, body)
		if err != nil {
			return err
		}
		resp.Body = io.NopCloser(bytes.NewReader(out))
		resp.ContentLength = int64(len(out))
		resp.Header.Set("Content-Length", strconv.Itoa(len(out)))
		resp.Header.Del("Transfer-Encoding")
		return nil
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
		r = r.WithContext(context.WithValue(r.Context(), l1ContextKey{}, route.l1))
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
