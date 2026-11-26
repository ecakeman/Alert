package config

import (
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ecakeman/alert/services/platform/db"
)

// Config 是 monitor-svc 的启动期配置；DATABASE_URL 必填（alert_monitor）。
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

	DatabaseURL         string
	DBMaxConns          int32
	DBMinConns          int32
	DBConnectTimeout    time.Duration
	DBMaxConnLifetime   time.Duration
	DBMaxConnIdleTime   time.Duration
	DBHealthcheckPeriod time.Duration

	KafkaBrokers       string
	KafkaPublishTopic  string
	KafkaConsumeTopics []string

	// 探测引擎（P3.4）。
	ProbeEnabled          bool
	ProbeWorkers          int
	ProbeQueueCapacity    int
	MonitorScanInterval   time.Duration
	ProbeDefaultTimeoutMS int
}

// LoadError 描述配置错误；只携带变量名与原因，不回显原值。
type LoadError struct {
	Var    string
	Reason string
}

func (e *LoadError) Error() string { return "config " + e.Var + ": " + e.Reason }

// Load 读取 monitor 配置表；缺 DATABASE_URL 或非法 URL 立即失败。
func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		ServiceName:        value(getenv, "SERVICE_NAME", "monitor-svc"),
		LogLevel:           value(getenv, "LOG_LEVEL", "info"),
		AppEnv:             value(getenv, "APP_ENV", "local"),
		HTTPAddr:           value(getenv, "HTTP_ADDR", ":8083"),
		DatabaseURL:        strings.TrimSpace(getenv("DATABASE_URL")),
		KafkaBrokers:       strings.TrimSpace(getenv("KAFKA_BROKERS")),
		KafkaPublishTopic:  value(getenv, "KAFKA_PUBLISH_TOPIC", "alertflow.monitor.events.v1"),
		KafkaConsumeTopics: splitList(value(getenv, "KAFKA_CONSUME_TOPICS", "alertflow.registry.events.v1")),
	}
	if cfg.DatabaseURL == "" {
		return Config{}, &LoadError{Var: "DATABASE_URL", Reason: "missing"}
	}
	u, err := url.Parse(cfg.DatabaseURL)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") {
		return Config{}, &LoadError{Var: "DATABASE_URL", Reason: "invalid url"}
	}

	if cfg.DBMaxConns, err = int32Value(getenv, "DB_MAX_CONNS", 10); err != nil {
		return Config{}, err
	}
	if cfg.DBMinConns, err = int32Value(getenv, "DB_MIN_CONNS", 1); err != nil {
		return Config{}, err
	}
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
	if cfg.DBConnectTimeout, err = duration(getenv, "DB_CONNECT_TIMEOUT", 5*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.DBMaxConnLifetime, err = duration(getenv, "DB_MAX_CONN_LIFETIME", 30*time.Minute); err != nil {
		return Config{}, err
	}
	if cfg.DBMaxConnIdleTime, err = duration(getenv, "DB_MAX_CONN_IDLE_TIME", 5*time.Minute); err != nil {
		return Config{}, err
	}
	if cfg.DBHealthcheckPeriod, err = duration(getenv, "DB_HEALTHCHECK_PERIOD", 30*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.MonitorScanInterval, err = duration(getenv, "MONITOR_SCAN_INTERVAL", time.Second); err != nil {
		return Config{}, err
	}
	if cfg.ProbeEnabled, err = boolValue(getenv, "PROBE_ENABLED", true); err != nil {
		return Config{}, err
	}
	if cfg.ProbeWorkers, err = intValue(getenv, "PROBE_WORKERS", 8); err != nil {
		return Config{}, err
	}
	if cfg.ProbeQueueCapacity, err = intValue(getenv, "PROBE_QUEUE_CAPACITY", 1024); err != nil {
		return Config{}, err
	}
	if cfg.ProbeDefaultTimeoutMS, err = intValue(getenv, "PROBE_DEFAULT_TIMEOUT_MS", 5000); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// DBConfig 返回 platform/db 的装配参数。
func (c Config) DBConfig() db.Config {
	return db.Config{
		URL:               c.DatabaseURL,
		MaxConns:          c.DBMaxConns,
		MinConns:          c.DBMinConns,
		ConnectTimeout:    c.DBConnectTimeout,
		MaxConnLifetime:   c.DBMaxConnLifetime,
		MaxConnIdleTime:   c.DBMaxConnIdleTime,
		HealthCheckPeriod: c.DBHealthcheckPeriod,
	}
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

func int32Value(getenv func(string) string, key string, fallback int32) (int32, error) {
	raw := strings.TrimSpace(getenv(key))
	if raw == "" {
		return fallback, nil
	}
	n, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || n < 0 {
		return 0, &LoadError{Var: key, Reason: "must be a non-negative integer"}
	}
	return int32(n), nil
}

func intValue(getenv func(string) string, key string, fallback int) (int, error) {
	raw := strings.TrimSpace(getenv(key))
	if raw == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 0, &LoadError{Var: key, Reason: "must be a positive integer"}
	}
	return n, nil
}

func boolValue(getenv func(string) string, key string, fallback bool) (bool, error) {
	raw := strings.TrimSpace(getenv(key))
	if raw == "" {
		return fallback, nil
	}
	b, err := strconv.ParseBool(raw)
	if err != nil {
		return false, &LoadError{Var: key, Reason: "must be a boolean"}
	}
	return b, nil
}

func splitList(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}