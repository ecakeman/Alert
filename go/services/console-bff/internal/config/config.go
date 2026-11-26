package config

import (
	"net/url"
	"os"
	"strings"
	"time"
)

// Config 是 console-bff 的启动期配置；无数据库，仅决定监听与下游地址。
type Config struct {
	ServiceName string
	LogLevel    string
	AppEnv      string

	HTTPAddr            string
	ReadyzTimeout       time.Duration
	HTTPReadTimeout     time.Duration
	HTTPWriteTimeout    time.Duration
	HTTPIdleTimeout     time.Duration
	HTTPShutdownTimeout time.Duration

	RegistryBaseURL  string
	MonitorBaseURL   string
	EpisodeBaseURL   string
	TaskBaseURL      string
	LegacyBaseURL    string
	UpstreamTimeout  time.Duration
	AggregateTimeout time.Duration
}

// LoadError 描述配置错误；只携带变量名与原因，不回显原值。
type LoadError struct {
	Var    string
	Reason string
}

func (e *LoadError) Error() string { return "config " + e.Var + ": " + e.Reason }

// Load 读取 BFF 配置表；DATABASE_URL 存在即 fail-fast（BFF 无库）。
func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		ServiceName:     value(getenv, "SERVICE_NAME", "console-bff"),
		LogLevel:        value(getenv, "LOG_LEVEL", "info"),
		AppEnv:          value(getenv, "APP_ENV", "local"),
		HTTPAddr:        value(getenv, "HTTP_ADDR", ":8080"),
		RegistryBaseURL: value(getenv, "REGISTRY_BASE_URL", "http://127.0.0.1:8081"),
		MonitorBaseURL:  value(getenv, "MONITOR_BASE_URL", "http://127.0.0.1:8081"),
		EpisodeBaseURL:  value(getenv, "EPISODE_BASE_URL", "http://127.0.0.1:8081"),
		TaskBaseURL:     value(getenv, "TASK_BASE_URL", "http://127.0.0.1:8081"),
		LegacyBaseURL:   value(getenv, "LEGACY_BASE_URL", "http://127.0.0.1:8084"),
	}

	if strings.TrimSpace(getenv("DATABASE_URL")) != "" {
		return Config{}, &LoadError{Var: "DATABASE_URL", Reason: "must not be set for console-bff"}
	}

	var err error
	if cfg.ReadyzTimeout, err = duration(getenv, "READYZ_TIMEOUT", 2*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.HTTPReadTimeout, err = duration(getenv, "HTTP_READ_TIMEOUT", 10*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.HTTPWriteTimeout, err = duration(getenv, "HTTP_WRITE_TIMEOUT", 15*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.HTTPIdleTimeout, err = duration(getenv, "HTTP_IDLE_TIMEOUT", 60*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.HTTPShutdownTimeout, err = duration(getenv, "HTTP_SHUTDOWN_TIMEOUT", 10*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.UpstreamTimeout, err = duration(getenv, "UPSTREAM_TIMEOUT", 3*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.AggregateTimeout, err = duration(getenv, "AGGREGATE_TIMEOUT", 800*time.Millisecond); err != nil {
		return Config{}, err
	}

	for name, raw := range map[string]string{
		"REGISTRY_BASE_URL": cfg.RegistryBaseURL,
		"MONITOR_BASE_URL":  cfg.MonitorBaseURL,
		"EPISODE_BASE_URL":  cfg.EpisodeBaseURL,
		"TASK_BASE_URL":     cfg.TaskBaseURL,
	} {
		if err := validateBaseURL(name, raw); err != nil {
			return Config{}, err
		}
	}
	return cfg, nil
}

func validateBaseURL(name, raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return &LoadError{Var: name, Reason: "must be http(s) base url"}
	}
	return nil
}

func value(getenv func(string) string, key, fallback string) string {
	if v := strings.TrimSpace(getenv(key)); v != "" {
		return v
	}
	return fallback
}

func duration(getenv func(string) string, key string, fallback time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(getenv(key))
	if raw == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return 0, &LoadError{Var: key, Reason: "must be a positive duration"}
	}
	return d, nil
}

// Ensure os is imported when Load is used with os.Getenv directly by callers.
var _ = os.Getenv
