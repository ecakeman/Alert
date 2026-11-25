package logging

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

// Config 是日志装配的输入；Output 为空时写 stdout。
type Config struct {
	Service string
	Level   string
	JSON    bool
	Output  io.Writer
}

// LoadError 描述日志配置错误；只携带变量名与原因，不回显原值。
type LoadError struct {
	Var    string
	Reason string
}

func (e *LoadError) Error() string { return "config " + e.Var + ": " + e.Reason }

// ParseLevel 解析日志级别；空串按 info 处理。
func ParseLevel(raw string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "info":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, &LoadError{Var: "LOG_LEVEL", Reason: "unsupported value"}
	}
}

// New 构建带 service 字段的结构化 logger。
func New(cfg Config) (*slog.Logger, error) {
	if strings.TrimSpace(cfg.Service) == "" {
		return nil, &LoadError{Var: "SERVICE_NAME", Reason: "must not be empty"}
	}
	level, err := ParseLevel(cfg.Level)
	if err != nil {
		return nil, err
	}
	out := cfg.Output
	if out == nil {
		out = os.Stdout
	}
	opts := &slog.HandlerOptions{Level: level}
	var handler slog.Handler
	if cfg.JSON {
		handler = slog.NewJSONHandler(out, opts)
	} else {
		handler = slog.NewTextHandler(out, opts)
	}
	return slog.New(handler).With("service", cfg.Service), nil
}

// WithTrace 返回附加 trace_id 的 logger；空 trace 原样返回。
func WithTrace(logger *slog.Logger, traceID string) *slog.Logger {
	if logger == nil || traceID == "" {
		return logger
	}
	return logger.With("trace_id", traceID)
}

// WithEvent 返回附加 event_id 的 logger；空 event 原样返回。
func WithEvent(logger *slog.Logger, eventID string) *slog.Logger {
	if logger == nil || eventID == "" {
		return logger
	}
	return logger.With("event_id", eventID)
}
