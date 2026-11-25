package httpx

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/ecakeman/alert/services/platform/apierr"
)

// Middleware 是标准 net/http 中间件。
type Middleware func(http.Handler) http.Handler

// Chain 按声明顺序包裹 handler（第一个最外层）。
func Chain(h http.Handler, middlewares ...Middleware) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return h
}

// Trace 读取/生成追踪 ID，写入 context 与响应头；上游缺失或不合法时重新生成。
func Trace(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			traceID := r.Header.Get(HeaderTraceID)
			if !ValidTraceID(traceID) {
				traceID = NewTraceID()
			}
			w.Header().Set(HeaderTraceID, traceID)
			ctx := WithTraceID(r.Context(), traceID)
			next.ServeHTTP(w, r.WithContext(ctx))
			if logger != nil {
				logger.Debug("request completed", "trace_id", traceID, "method", r.Method, "path", r.URL.Path)
			}
		})
	}
}

// AccessLog 记录方法、路径、状态码、耗时与 trace_id。
func AccessLog(logger *slog.Logger) Middleware {
	if logger == nil {
		return func(next http.Handler) http.Handler { return next }
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(recorder, r)
			logger.Info("access",
				"trace_id", TraceIDFrom(r.Context()),
				"method", r.Method,
				"path", r.URL.Path,
				"status", recorder.status,
				"duration_ms", time.Since(start).Milliseconds(),
			)
		})
	}
}

// Recovery 把 panic 转为 500 JSON，记录 trace_id。
func Recovery(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if recovered := recover(); recovered != nil {
					if logger != nil {
						logger.Error("panic recovered",
							"trace_id", TraceIDFrom(r.Context()),
							"path", r.URL.Path,
							"panic", recovered,
						)
					}
					apierr.Write(w, apierr.New(apierr.CodeInternal, "internal error"))
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// Timeout 为请求 context 设置超时；超时处理由业务 handler 或客户端负责。
func Timeout(d time.Duration) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), d)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}
