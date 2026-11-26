package config

import (
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ecakeman/alert/services/platform/db"
)

// Config 是 task-orchestrator 的启动期配置；DATABASE_URL 必填（alert_task）。
type Config struct {
	ServiceName string
	LogLevel    string
	AppEnv      string

	HTTPAddr            string
	GRPCAddr            string
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

	EpisodeBaseURL      string
	RegistryBaseURL     string
	MonitorBaseURL      string
	UpstreamTimeout     time.Duration
	AgentGRPCTarget     string
	ArtifactStorageRoot string
	ReconcileInterval   time.Duration
	TaskLease           time.Duration
	TaskMaxAttempts     int
}

type LoadError struct {
	Var    string
	Reason string
}

func (e *LoadError) Error() string { return "config " + e.Var + ": " + e.Reason }

func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		ServiceName:         value(getenv, "SERVICE_NAME", "task-orchestrator"),
		LogLevel:            value(getenv, "LOG_LEVEL", "info"),
		AppEnv:              value(getenv, "APP_ENV", "local"),
		HTTPAddr:            value(getenv, "HTTP_ADDR", ":8084"),
		GRPCAddr:            value(getenv, "GRPC_ADDR", ":9090"),
		DatabaseURL:         strings.TrimSpace(getenv("DATABASE_URL")),
		KafkaBrokers:        strings.TrimSpace(getenv("KAFKA_BROKERS")),
		KafkaPublishTopic:   value(getenv, "KAFKA_PUBLISH_TOPIC", "alertflow.task.events.v1"),
		KafkaConsumeTopics:  splitList(value(getenv, "KAFKA_CONSUME_TOPICS", "alertflow.episode.events.v1,alertflow.task.events.v1")),
		EpisodeBaseURL:      value(getenv, "EPISODE_BASE_URL", "http://127.0.0.1:8082"),
		RegistryBaseURL:     value(getenv, "REGISTRY_BASE_URL", "http://127.0.0.1:8081"),
		MonitorBaseURL:      value(getenv, "MONITOR_BASE_URL", "http://127.0.0.1:8083"),
		AgentGRPCTarget:     value(getenv, "AGENT_GRPC_TARGET", "127.0.0.1:50052"),
		ArtifactStorageRoot: value(getenv, "ARTIFACT_STORAGE_ROOT", "/data/artifacts"),
	}
	if cfg.DatabaseURL == "" {
		return Config{}, &LoadError{Var: "DATABASE_URL", Reason: "missing"}
	}
	u, err := url.Parse(cfg.DatabaseURL)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") {
		return Config{}, &LoadError{Var: "DATABASE_URL", Reason: "invalid url"}
	}
	if !strings.HasPrefix(cfg.GRPCAddr, ":") {
		return Config{}, &LoadError{Var: "GRPC_ADDR", Reason: "must be :port"}
	}

	var perr error
	if cfg.DBMaxConns, perr = int32Value(getenv, "DB_MAX_CONNS", 10); perr != nil {
		return Config{}, perr
	}
	if cfg.DBMinConns, perr = int32Value(getenv, "DB_MIN_CONNS", 1); perr != nil {
		return Config{}, perr
	}
	durations := []struct {
		key      string
		fallback time.Duration
		target   *time.Duration
	}{
		{"READYZ_TIMEOUT", 2 * time.Second, &cfg.ReadyzTimeout},
		{"HTTP_READ_TIMEOUT", 10 * time.Second, &cfg.HTTPReadTimeout},
		{"HTTP_WRITE_TIMEOUT", 15 * time.Second, &cfg.HTTPWriteTimeout},
		{"HTTP_IDLE_TIMEOUT", 60 * time.Second, &cfg.HTTPIdleTimeout},
		{"HTTP_SHUTDOWN_TIMEOUT", 10 * time.Second, &cfg.HTTPShutdownTimeout},
		{"DB_CONNECT_TIMEOUT", 5 * time.Second, &cfg.DBConnectTimeout},
		{"DB_MAX_CONN_LIFETIME", 30 * time.Minute, &cfg.DBMaxConnLifetime},
		{"DB_MAX_CONN_IDLE_TIME", 5 * time.Minute, &cfg.DBMaxConnIdleTime},
		{"DB_HEALTHCHECK_PERIOD", 30 * time.Second, &cfg.DBHealthcheckPeriod},
		{"UPSTREAM_TIMEOUT", 3 * time.Second, &cfg.UpstreamTimeout},
		{"TASK_RECONCILE_INTERVAL", 30 * time.Second, &cfg.ReconcileInterval},
		{"TASK_LEASE", 30 * time.Second, &cfg.TaskLease},
	}
	for _, d := range durations {
		v, err := duration(getenv, d.key, d.fallback)
		if err != nil {
			return Config{}, err
		}
		*d.target = v
	}
	if cfg.TaskMaxAttempts, perr = intValue(getenv, "TASK_MAX_ATTEMPTS", 3); perr != nil {
		return Config{}, perr
	}
	return cfg, nil
}

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
	if err != nil || n < 1 {
		return 0, &LoadError{Var: key, Reason: "must be a positive integer"}
	}
	return n, nil
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