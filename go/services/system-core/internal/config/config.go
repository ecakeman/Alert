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
	KafkaMonitorTopic      string
	ProbeEnabled           bool
	RemoteRegistry         bool
	RegistryBaseURL        string
	EpisodeBaseURL         string
	RemoteMonitor          bool
	MonitorBaseURL         string
	UpstreamTimeout        time.Duration
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

// envSpec 是描述项：env 为变量名（= LoadError.Var），apply 在 raw 非空时解析并赋值、空串跳过（保持默认值）。
type envSpec struct {
	env   string
	apply func(c *Config, raw string) error
}

// 四个泛型解析构造器：错误 Reason 文本由 parse 单点产生，天然冻结。
func strSpec(env string, set func(*Config, string)) envSpec {
	return envSpec{env: env, apply: func(c *Config, raw string) error {
		if raw == "" {
			return nil
		}
		set(c, raw)
		return nil
	}}
}

func boolSpec(env string, set func(*Config, bool)) envSpec {
	return envSpec{env: env, apply: func(c *Config, raw string) error {
		if raw == "" {
			return nil
		}
		b, err := parseBool(raw, env)
		if err != nil {
			return err
		}
		set(c, b)
		return nil
	}}
}

func intSpec(env string, set func(*Config, int)) envSpec {
	return envSpec{env: env, apply: func(c *Config, raw string) error {
		if raw == "" {
			return nil
		}
		n, err := parseInt(raw, env)
		if err != nil {
			return err
		}
		set(c, n)
		return nil
	}}
}

func durSpec(env string, set func(*Config, time.Duration)) envSpec {
	return envSpec{env: env, apply: func(c *Config, raw string) error {
		if raw == "" {
			return nil
		}
		d, err := parseDuration(raw, env)
		if err != nil {
			return err
		}
		set(c, d)
		return nil
	}}
}

// 分组描述项表（分组仅为表组织；DBMinConns 亦有 DB_MIN_CONNS 覆盖项，与 Validate 范围校验并存）。
var (
	sharedSpecs = []envSpec{
		strSpec("LOG_LEVEL", func(c *Config, v string) { c.LogLevel = v }),
		strSpec("APP_ENV", func(c *Config, v string) { c.AppEnv = v }),
		strSpec("DATABASE_URL", func(c *Config, v string) { c.DatabaseURL = v }),
		intSpec("DB_MAX_CONNS", func(c *Config, v int) { c.DBMaxConns = v }),
		intSpec("DB_MIN_CONNS", func(c *Config, v int) { c.DBMinConns = v }),
		durSpec("DB_CONNECT_TIMEOUT", func(c *Config, v time.Duration) { c.DBConnectTimeout = v }),
		durSpec("DB_MAX_CONN_LIFETIME", func(c *Config, v time.Duration) { c.DBMaxConnLifetime = v }),
		durSpec("DB_MAX_CONN_IDLE_TIME", func(c *Config, v time.Duration) { c.DBMaxConnIdleTime = v }),
		durSpec("DB_HEALTHCHECK_PERIOD", func(c *Config, v time.Duration) { c.DBHealthcheckPeriod = v }),
		durSpec("READYZ_TIMEOUT", func(c *Config, v time.Duration) { c.ReadyzTimeout = v }),
		durSpec("HTTP_READ_TIMEOUT", func(c *Config, v time.Duration) { c.HTTPReadTimeout = v }),
		durSpec("HTTP_WRITE_TIMEOUT", func(c *Config, v time.Duration) { c.HTTPWriteTimeout = v }),
		durSpec("HTTP_IDLE_TIMEOUT", func(c *Config, v time.Duration) { c.HTTPIdleTimeout = v }),
		durSpec("HTTP_SHUTDOWN_TIMEOUT", func(c *Config, v time.Duration) { c.HTTPShutdownTimeout = v }),
	}

	apiSpecs = []envSpec{
		strSpec("HTTP_ADDR", func(c *Config, v string) { c.HTTPAddr = v }),
		boolSpec("MIGRATE_ON_START", func(c *Config, v bool) { c.MigrateOnStart = v }),
	}

	workerSpecs = []envSpec{
		intSpec("TASK_MAX_ATTEMPTS", func(c *Config, v int) { c.TaskMaxAttempts = v }),
		durSpec("OWNERSHIP_TTL", func(c *Config, v time.Duration) { c.OwnershipTTL = v }),
		boolSpec("OUTBOX_PUBLISHER_ENABLED", func(c *Config, v bool) { c.OutboxPublisherEnabled = v }),
		durSpec("OUTBOX_CLAIM_LEASE", func(c *Config, v time.Duration) { c.OutboxClaimLease = v }),
		intSpec("OUTBOX_BATCH", func(c *Config, v int) { c.OutboxBatch = v }),
		boolSpec("REMOTE_REGISTRY", func(c *Config, v bool) { c.RemoteRegistry = v }),
		strSpec("REGISTRY_BASE_URL", func(c *Config, v string) { c.RegistryBaseURL = v }),
		boolSpec("REMOTE_MONITOR", func(c *Config, v bool) { c.RemoteMonitor = v }),
		strSpec("MONITOR_BASE_URL", func(c *Config, v string) { c.MonitorBaseURL = v }),
		strSpec("EPISODE_BASE_URL", func(c *Config, v string) { c.EpisodeBaseURL = v }),
		durSpec("UPSTREAM_TIMEOUT", func(c *Config, v time.Duration) { c.UpstreamTimeout = v }),
		strSpec("KAFKA_BROKERS", func(c *Config, v string) { c.KafkaBrokers = v }),
		strSpec("KAFKA_TOPIC", func(c *Config, v string) { c.KafkaTopic = v }),
		strSpec("KAFKA_MONITOR_TOPIC", func(c *Config, v string) { c.KafkaMonitorTopic = v }),
		boolSpec("PROBE_ENABLED", func(c *Config, v bool) { c.ProbeEnabled = v }),
		intSpec("PROBE_WORKERS", func(c *Config, v int) { c.ProbeWorkers = v }),
		intSpec("PROBE_QUEUE_CAPACITY", func(c *Config, v int) { c.ProbeQueueCapacity = v }),
		strSpec("AGENT_GRPC_TARGET", func(c *Config, v string) { c.AgentGRPCTarget = v }),
		strSpec("GRPC_ADDR", func(c *Config, v string) { c.GRPCAddr = v }),
		durSpec("AGENT_GRPC_TIMEOUT", func(c *Config, v time.Duration) { c.AgentGRPCTimeout = v }),
		durSpec("OUTBOX_POLL_INTERVAL", func(c *Config, v time.Duration) { c.OutboxPollInterval = v }),
		durSpec("TASK_RETRY_DELAY", func(c *Config, v time.Duration) { c.TaskRetryDelay = v }),
		durSpec("TASK_DEADLINE", func(c *Config, v time.Duration) { c.TaskDeadline = v }),
		durSpec("PROBE_SHUTDOWN_TIMEOUT", func(c *Config, v time.Duration) { c.ProbeShutdownTimeout = v }),
		durSpec("TASK_LEASE", func(c *Config, v time.Duration) { c.TaskLease = v }),
	}
)

// specByName 汇总三组表供 loadOrder 查找（同一变量仅登记一次）。
var specByName = func() map[string]envSpec {
	m := make(map[string]envSpec)
	for _, s := range sharedSpecs {
		m[s.env] = s
	}
	for _, s := range apiSpecs {
		m[s.env] = s
	}
	for _, s := range workerSpecs {
		m[s.env] = s
	}
	return m
}()

// loadOrder 逐项对齐原 Load 的变量书写顺序：三组表为交错归属，组间拼接无法直接复原原序，
// 故以有序名称列表驱动遍历，保证任一非法输入的首个 LoadError 与历史行为逐字节一致。
var loadOrder = []string{
	"HTTP_ADDR", "LOG_LEVEL", "APP_ENV", "DATABASE_URL", "MIGRATE_ON_START",
	"TASK_MAX_ATTEMPTS", "OWNERSHIP_TTL", "OUTBOX_PUBLISHER_ENABLED", "OUTBOX_CLAIM_LEASE", "OUTBOX_BATCH",
	"KAFKA_BROKERS", "KAFKA_TOPIC", "PROBE_WORKERS", "PROBE_QUEUE_CAPACITY", "AGENT_GRPC_TARGET",
	"DB_MAX_CONNS", "DB_MIN_CONNS", "DB_CONNECT_TIMEOUT", "DB_MAX_CONN_LIFETIME", "DB_MAX_CONN_IDLE_TIME",
	"DB_HEALTHCHECK_PERIOD", "GRPC_ADDR", "AGENT_GRPC_TIMEOUT", "OUTBOX_POLL_INTERVAL", "TASK_RETRY_DELAY",
	"TASK_DEADLINE", "PROBE_SHUTDOWN_TIMEOUT", "READYZ_TIMEOUT", "HTTP_READ_TIMEOUT", "HTTP_WRITE_TIMEOUT",
	"HTTP_IDLE_TIMEOUT", "HTTP_SHUTDOWN_TIMEOUT", "TASK_LEASE",
	"KAFKA_MONITOR_TOPIC", "PROBE_ENABLED",
	"REMOTE_REGISTRY", "REGISTRY_BASE_URL", "UPSTREAM_TIMEOUT",
	"REMOTE_MONITOR", "MONITOR_BASE_URL", "EPISODE_BASE_URL",
}

// Load 从 lookup 读取配置：先填默认值，再按表解析（首个非法项即返回），最后校验。
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
		KafkaMonitorTopic:      "alertflow.monitor.events.v1",
		ProbeEnabled:           true,
		RegistryBaseURL:        "http://127.0.0.1:8081",
		MonitorBaseURL:         "http://127.0.0.1:8083",
		EpisodeBaseURL:         "http://127.0.0.1:8082",
		UpstreamTimeout:        3 * time.Second,
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

	for _, name := range loadOrder {
		if err := specByName[name].apply(&c, lookup(name)); err != nil {
			return Config{}, err
		}
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
	if c.KafkaMonitorTopic == "" {
		return &LoadError{Var: "KAFKA_MONITOR_TOPIC", Reason: "must not be empty"}
	}
	if _, _, err := net.SplitHostPort(c.GRPCAddr); err != nil {
		return &LoadError{Var: "GRPC_ADDR", Reason: "must be host:port"}
	}
	return nil
}

// ValidateRuntime 强制运行期必需项：DATABASE_URL；publisher 启用时 Kafka broker。
// AGENT_GRPC_TARGET 的必需性留到 B7 装配时加入。
func (c Config) ValidateRuntime() error {
	if c.DatabaseURL == "" {
		return &LoadError{Var: "DATABASE_URL", Reason: "missing"}
	}
	u, err := url.Parse(c.DatabaseURL)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") {
		return &LoadError{Var: "DATABASE_URL", Reason: "invalid url"}
	}
	if c.OutboxPublisherEnabled && c.KafkaBrokers == "" {
		return &LoadError{Var: "KAFKA_BROKERS", Reason: "missing"}
	}
	return nil
}

// ValidateAPIRuntime 校验 api 角色必需项：gRPC 监听地址与租约上限关系。
func (c Config) ValidateAPIRuntime() error {
	if _, _, err := net.SplitHostPort(c.GRPCAddr); err != nil {
		return &LoadError{Var: "GRPC_ADDR", Reason: "must be host:port"}
	}
	if c.TaskLease > c.OwnershipTTL {
		return &LoadError{Var: "TASK_LEASE", Reason: "must not exceed OWNERSHIP_TTL"}
	}
	return nil
}

// ValidateWorkerRuntime 校验 worker 角色必需项：Agent 地址与 Kafka 前置。
func (c Config) ValidateWorkerRuntime() error {
	if c.AgentGRPCTarget == "" {
		return &LoadError{Var: "AGENT_GRPC_TARGET", Reason: "missing"}
	}
	if _, _, err := net.SplitHostPort(c.AgentGRPCTarget); err != nil {
		return &LoadError{Var: "AGENT_GRPC_TARGET", Reason: "must be host:port"}
	}
	if c.OutboxPublisherEnabled && c.KafkaBrokers == "" {
		return &LoadError{Var: "KAFKA_BROKERS", Reason: "missing"}
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
