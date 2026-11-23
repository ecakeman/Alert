package config

import (
	"net"
	"net/url"
	"strconv"
	"time"
)

// Config 是启动期的值对象配置，全部为值类型。
type Config struct {
	HTTPAddr            string
	HTTPReadTimeout     time.Duration
	HTTPWriteTimeout    time.Duration
	HTTPIdleTimeout     time.Duration
	HTTPShutdownTimeout time.Duration
	LogLevel            string
	AppEnv              string

	DatabaseURL            string
	MigrateOnStart         bool
	TaskLease              time.Duration
	TaskMaxAttempts        int
	OwnershipTTL           time.Duration
	OutboxPublisherEnabled bool
	OutboxClaimLease       time.Duration
	OutboxBatch            int
	KafkaBrokers           string
	KafkaTopic             string
	ProbeWorkers           int
	ProbeQueueCapacity     int
	AgentGRPCTarget        string

	DBMaxConns           int
	DBMinConns           int
	DBConnectTimeout     time.Duration
	DBMaxConnLifetime    time.Duration
	DBMaxConnIdleTime    time.Duration
	DBHealthcheckPeriod  time.Duration
	GRPCAddr             string
	AgentGRPCTimeout     time.Duration
	OutboxPollInterval   time.Duration
	TaskRetryDelay       time.Duration
	TaskDeadline         time.Duration
	ProbeShutdownTimeout time.Duration
	ReadyzTimeout        time.Duration
}

// LoadError 描述配置错误；只携带变量名与原因，不回显原值。
type LoadError struct {
	Var    string
	Reason string
}

func (e *LoadError) Error() string { return "config " + e.Var + ": " + e.Reason }

var validLogLevels = map[string]bool{"debug": true, "info": true, "warn": true, "error": true}

// Load 从 lookup 读取配置：先填默认值，再解析，最后校验。
// 保持零外部依赖（不读文件、不连库），可独立单测。
func Load(lookup func(string) string) (Config, error) {
	c := Config{
		HTTPAddr:               ":8080",
		HTTPReadTimeout:        10 * time.Second,
		HTTPWriteTimeout:       15 * time.Second,
		HTTPIdleTimeout:        60 * time.Second,
		HTTPShutdownTimeout:    10 * time.Second,
		LogLevel:               "info",
		AppEnv:                 "local",
		MigrateOnStart:         false,
		TaskLease:              30 * time.Second,
		TaskMaxAttempts:        3,
		OwnershipTTL:           60 * time.Second,
		OutboxPublisherEnabled: true,
		OutboxClaimLease:       30 * time.Second,
		OutboxBatch:            16,
		KafkaTopic:             "alertflow.system.events.v1",
		ProbeWorkers:           8,
		ProbeQueueCapacity:     1024,
		DBMaxConns:             10,
		DBMinConns:             1,
		DBConnectTimeout:       5 * time.Second,
		DBMaxConnLifetime:      30 * time.Minute,
		DBMaxConnIdleTime:      5 * time.Minute,
		DBHealthcheckPeriod:    30 * time.Second,
		GRPCAddr:               ":9090",
		AgentGRPCTimeout:       5 * time.Second,
		OutboxPollInterval:     500 * time.Millisecond,
		TaskRetryDelay:         30 * time.Second,
		TaskDeadline:           10 * time.Minute,
		ProbeShutdownTimeout:   5 * time.Second,
		ReadyzTimeout:          2 * time.Second,
	}

	if v := lookup("HTTP_ADDR"); v != "" {
		c.HTTPAddr = v
	}
	if v := lookup("LOG_LEVEL"); v != "" {
		c.LogLevel = v
	}
	if v := lookup("APP_ENV"); v != "" {
		c.AppEnv = v
	}

	if v := lookup("DATABASE_URL"); v != "" {
		c.DatabaseURL = v
	}
	if v := lookup("MIGRATE_ON_START"); v != "" {
		b, err := parseBool(v, "MIGRATE_ON_START")
		if err != nil {
			return Config{}, err
		}
		c.MigrateOnStart = b
	}

	if v := lookup("TASK_MAX_ATTEMPTS"); v != "" {
		n, err := parseInt(v, "TASK_MAX_ATTEMPTS")
		if err != nil {
			return Config{}, err
		}
		c.TaskMaxAttempts = n
	}
	if v := lookup("OWNERSHIP_TTL"); v != "" {
		d, err := parseDuration(v, "OWNERSHIP_TTL")
		if err != nil {
			return Config{}, err
		}
		c.OwnershipTTL = d
	}
	if v := lookup("OUTBOX_PUBLISHER_ENABLED"); v != "" {
		b, err := parseBool(v, "OUTBOX_PUBLISHER_ENABLED")
		if err != nil {
			return Config{}, err
		}
		c.OutboxPublisherEnabled = b
	}
	if v := lookup("OUTBOX_CLAIM_LEASE"); v != "" {
		d, err := parseDuration(v, "OUTBOX_CLAIM_LEASE")
		if err != nil {
			return Config{}, err
		}
		c.OutboxClaimLease = d
	}
	if v := lookup("OUTBOX_BATCH"); v != "" {
		n, err := parseInt(v, "OUTBOX_BATCH")
		if err != nil {
			return Config{}, err
		}
		c.OutboxBatch = n
	}
	if v := lookup("KAFKA_BROKERS"); v != "" {
		c.KafkaBrokers = v
	}
	if v := lookup("KAFKA_TOPIC"); v != "" {
		c.KafkaTopic = v
	}
	if v := lookup("PROBE_WORKERS"); v != "" {
		n, err := parseInt(v, "PROBE_WORKERS")
		if err != nil {
			return Config{}, err
		}
		c.ProbeWorkers = n
	}
	if v := lookup("PROBE_QUEUE_CAPACITY"); v != "" {
		n, err := parseInt(v, "PROBE_QUEUE_CAPACITY")
		if err != nil {
			return Config{}, err
		}
		c.ProbeQueueCapacity = n
	}
	if v := lookup("AGENT_GRPC_TARGET"); v != "" {
		c.AgentGRPCTarget = v
	}

	if v := lookup("DB_MAX_CONNS"); v != "" {
		n, err := parseInt(v, "DB_MAX_CONNS")
		if err != nil {
			return Config{}, err
		}
		c.DBMaxConns = n
	}
	if v := lookup("DB_MIN_CONNS"); v != "" {
		n, err := parseInt(v, "DB_MIN_CONNS")
		if err != nil {
			return Config{}, err
		}
		c.DBMinConns = n
	}
	if v := lookup("DB_CONNECT_TIMEOUT"); v != "" {
		d, err := parseDuration(v, "DB_CONNECT_TIMEOUT")
		if err != nil {
			return Config{}, err
		}
		c.DBConnectTimeout = d
	}
	if v := lookup("DB_MAX_CONN_LIFETIME"); v != "" {
		d, err := parseDuration(v, "DB_MAX_CONN_LIFETIME")
		if err != nil {
			return Config{}, err
		}
		c.DBMaxConnLifetime = d
	}
	if v := lookup("DB_MAX_CONN_IDLE_TIME"); v != "" {
		d, err := parseDuration(v, "DB_MAX_CONN_IDLE_TIME")
		if err != nil {
			return Config{}, err
		}
		c.DBMaxConnIdleTime = d
	}
	if v := lookup("DB_HEALTHCHECK_PERIOD"); v != "" {
		d, err := parseDuration(v, "DB_HEALTHCHECK_PERIOD")
		if err != nil {
			return Config{}, err
		}
		c.DBHealthcheckPeriod = d
	}
	if v := lookup("GRPC_ADDR"); v != "" {
		c.GRPCAddr = v
	}
	if v := lookup("AGENT_GRPC_TIMEOUT"); v != "" {
		d, err := parseDuration(v, "AGENT_GRPC_TIMEOUT")
		if err != nil {
			return Config{}, err
		}
		c.AgentGRPCTimeout = d
	}
	if v := lookup("OUTBOX_POLL_INTERVAL"); v != "" {
		d, err := parseDuration(v, "OUTBOX_POLL_INTERVAL")
		if err != nil {
			return Config{}, err
		}
		c.OutboxPollInterval = d
	}
	if v := lookup("TASK_RETRY_DELAY"); v != "" {
		d, err := parseDuration(v, "TASK_RETRY_DELAY")
		if err != nil {
			return Config{}, err
		}
		c.TaskRetryDelay = d
	}
	if v := lookup("TASK_DEADLINE"); v != "" {
		d, err := parseDuration(v, "TASK_DEADLINE")
		if err != nil {
			return Config{}, err
		}
		c.TaskDeadline = d
	}
	if v := lookup("PROBE_SHUTDOWN_TIMEOUT"); v != "" {
		d, err := parseDuration(v, "PROBE_SHUTDOWN_TIMEOUT")
		if err != nil {
			return Config{}, err
		}
		c.ProbeShutdownTimeout = d
	}
	if v := lookup("READYZ_TIMEOUT"); v != "" {
		d, err := parseDuration(v, "READYZ_TIMEOUT")
		if err != nil {
			return Config{}, err
		}
		c.ReadyzTimeout = d
	}

	var err error
	if c.HTTPReadTimeout, err = parseDurOpt(lookup("HTTP_READ_TIMEOUT"), c.HTTPReadTimeout, "HTTP_READ_TIMEOUT"); err != nil {
		return Config{}, err
	}
	if c.HTTPWriteTimeout, err = parseDurOpt(lookup("HTTP_WRITE_TIMEOUT"), c.HTTPWriteTimeout, "HTTP_WRITE_TIMEOUT"); err != nil {
		return Config{}, err
	}
	if c.HTTPIdleTimeout, err = parseDurOpt(lookup("HTTP_IDLE_TIMEOUT"), c.HTTPIdleTimeout, "HTTP_IDLE_TIMEOUT"); err != nil {
		return Config{}, err
	}
	if c.HTTPShutdownTimeout, err = parseDurOpt(lookup("HTTP_SHUTDOWN_TIMEOUT"), c.HTTPShutdownTimeout, "HTTP_SHUTDOWN_TIMEOUT"); err != nil {
		return Config{}, err
	}
	if c.TaskLease, err = parseDurOpt(lookup("TASK_LEASE"), c.TaskLease, "TASK_LEASE"); err != nil {
		return Config{}, err
	}

	if err := c.Validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

func parseBool(raw, name string) (bool, error) {
	b, err := strconv.ParseBool(raw)
	if err != nil {
		return false, &LoadError{Var: name, Reason: "invalid bool"}
	}
	return b, nil
}

func parseInt(raw, name string) (int, error) {
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, &LoadError{Var: name, Reason: "invalid int"}
	}
	return n, nil
}

func parseDuration(raw, name string) (time.Duration, error) {
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, &LoadError{Var: name, Reason: "invalid duration"}
	}
	return d, nil
}

func parseDurOpt(raw string, def time.Duration, name string) (time.Duration, error) {
	if raw == "" {
		return def, nil
	}
	return parseDuration(raw, name)
}

// Validate 校验配置合法性；host 可为空（":8080" 合法）。
func (c Config) Validate() error {
	_, port, err := net.SplitHostPort(c.HTTPAddr)
	if err != nil {
		return &LoadError{Var: "HTTP_ADDR", Reason: "must be host:port"}
	}
	if p, err := strconv.Atoi(port); err != nil || p < 1 || p > 65535 {
		return &LoadError{Var: "HTTP_ADDR", Reason: "port out of range"}
	}

	if err := validateTimeout("HTTP_READ_TIMEOUT", c.HTTPReadTimeout, 300*time.Second); err != nil {
		return err
	}
	if err := validateTimeout("HTTP_WRITE_TIMEOUT", c.HTTPWriteTimeout, 300*time.Second); err != nil {
		return err
	}
	if err := validateTimeout("HTTP_IDLE_TIMEOUT", c.HTTPIdleTimeout, 300*time.Second); err != nil {
		return err
	}
	if err := validateTimeout("HTTP_SHUTDOWN_TIMEOUT", c.HTTPShutdownTimeout, 30*time.Second); err != nil {
		return err
	}

	if !validLogLevels[c.LogLevel] {
		return &LoadError{Var: "LOG_LEVEL", Reason: "unsupported value"}
	}
	if c.AppEnv == "" {
		return &LoadError{Var: "APP_ENV", Reason: "must not be empty"}
	}

	if err := validateRange("TASK_LEASE", int64(c.TaskLease), 1, int64(10*time.Minute)); err != nil {
		return err
	}
	if err := validateRange("TASK_MAX_ATTEMPTS", int64(c.TaskMaxAttempts), 1, 10); err != nil {
		return err
	}
	if err := validateRange("OWNERSHIP_TTL", int64(c.OwnershipTTL), 1, int64(time.Hour)); err != nil {
		return err
	}
	if err := validateRange("OUTBOX_CLAIM_LEASE", int64(c.OutboxClaimLease), 1, int64(10*time.Minute)); err != nil {
		return err
	}
	if err := validateRange("OUTBOX_BATCH", int64(c.OutboxBatch), 1, 256); err != nil {
		return err
	}
	if err := validateRange("PROBE_WORKERS", int64(c.ProbeWorkers), 1, 64); err != nil {
		return err
	}
	if err := validateRange("PROBE_QUEUE_CAPACITY", int64(c.ProbeQueueCapacity), 64, 65536); err != nil {
		return err
	}
	if err := validateRange("DB_MAX_CONNS", int64(c.DBMaxConns), 1, 100); err != nil {
		return err
	}
	if c.DBMinConns < 0 || c.DBMinConns > c.DBMaxConns {
		return &LoadError{Var: "DB_MIN_CONNS", Reason: "out of range"}
	}

	if err := validatePositive("DB_CONNECT_TIMEOUT", c.DBConnectTimeout); err != nil {
		return err
	}
	if err := validatePositive("DB_MAX_CONN_LIFETIME", c.DBMaxConnLifetime); err != nil {
		return err
	}
	if err := validatePositive("DB_MAX_CONN_IDLE_TIME", c.DBMaxConnIdleTime); err != nil {
		return err
	}
	if err := validatePositive("DB_HEALTHCHECK_PERIOD", c.DBHealthcheckPeriod); err != nil {
		return err
	}
	if err := validatePositive("AGENT_GRPC_TIMEOUT", c.AgentGRPCTimeout); err != nil {
		return err
	}
	if err := validatePositive("OUTBOX_POLL_INTERVAL", c.OutboxPollInterval); err != nil {
		return err
	}
	if err := validatePositive("TASK_RETRY_DELAY", c.TaskRetryDelay); err != nil {
		return err
	}
	if err := validatePositive("TASK_DEADLINE", c.TaskDeadline); err != nil {
		return err
	}
	if err := validatePositive("PROBE_SHUTDOWN_TIMEOUT", c.ProbeShutdownTimeout); err != nil {
		return err
	}
	if err := validatePositive("READYZ_TIMEOUT", c.ReadyzTimeout); err != nil {
		return err
	}

	if c.KafkaTopic == "" {
		return &LoadError{Var: "KAFKA_TOPIC", Reason: "must not be empty"}
	}
	if _, _, err := net.SplitHostPort(c.GRPCAddr); err != nil {
		return &LoadError{Var: "GRPC_ADDR", Reason: "must be host:port"}
	}
	return nil
}

// ValidateRuntime 强制运行期必需项；当前仅 DATABASE_URL。
// Kafka/Agent 的必需性检查留到对应块装配时加入。
func (c Config) ValidateRuntime() error {
	if c.DatabaseURL == "" {
		return &LoadError{Var: "DATABASE_URL", Reason: "missing"}
	}
	u, err := url.Parse(c.DatabaseURL)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") {
		return &LoadError{Var: "DATABASE_URL", Reason: "invalid url"}
	}
	return nil
}

func validateTimeout(name string, d, max time.Duration) error {
	if d <= 0 {
		return &LoadError{Var: name, Reason: "must be > 0"}
	}
	if d > max {
		return &LoadError{Var: name, Reason: "exceeds upper bound"}
	}
	return nil
}

func validateRange(name string, v, min, max int64) error {
	if v < min || v > max {
		return &LoadError{Var: name, Reason: "out of range"}
	}
	return nil
}

func validatePositive(name string, d time.Duration) error {
	if d <= 0 {
		return &LoadError{Var: name, Reason: "must be > 0"}
	}
	return nil
}