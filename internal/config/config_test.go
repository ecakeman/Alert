package config

import (
	"strings"
	"testing"
	"time"
)

func lookupFrom(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadDefaults(t *testing.T) {
	c, err := Load(lookupFrom(nil))
	if err != nil {
		t.Fatalf("defaults should load: %v", err)
	}
	if c.HTTPAddr != ":8080" || c.LogLevel != "info" || c.AppEnv != "local" {
		t.Fatalf("unexpected defaults: %+v", c)
	}
	if c.HTTPReadTimeout != 10*time.Second || c.HTTPWriteTimeout != 15*time.Second ||
		c.HTTPIdleTimeout != 60*time.Second || c.HTTPShutdownTimeout != 10*time.Second {
		t.Fatalf("unexpected timeout defaults: %+v", c)
	}
}

// B0.2：26 项新增配置的默认值逐项断言。
func TestLoadDefaultsExtended(t *testing.T) {
	c, err := Load(lookupFrom(nil))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if c.DatabaseURL != "" {
		t.Fatal("DATABASE_URL must have no default")
	}
	if c.MigrateOnStart {
		t.Fatal("MIGRATE_ON_START default false")
	}
	if c.TaskLease != 30*time.Second || c.TaskMaxAttempts != 3 || c.OwnershipTTL != 60*time.Second {
		t.Fatalf("task defaults: %+v", c)
	}
	if !c.OutboxPublisherEnabled || c.OutboxClaimLease != 30*time.Second || c.OutboxBatch != 16 {
		t.Fatalf("outbox defaults: %+v", c)
	}
	if c.KafkaBrokers != "" || c.KafkaTopic != "alertflow.system.events.v1" {
		t.Fatalf("kafka defaults: %+v", c)
	}
	if c.ProbeWorkers != 8 || c.ProbeQueueCapacity != 1024 || c.AgentGRPCTarget != "" {
		t.Fatalf("probe defaults: %+v", c)
	}
	if c.DBMaxConns != 10 || c.DBMinConns != 1 || c.DBConnectTimeout != 5*time.Second ||
		c.DBMaxConnLifetime != 30*time.Minute || c.DBMaxConnIdleTime != 5*time.Minute ||
		c.DBHealthcheckPeriod != 30*time.Second {
		t.Fatalf("db defaults: %+v", c)
	}
	if c.GRPCAddr != ":9090" || c.AgentGRPCTimeout != 5*time.Second ||
		c.OutboxPollInterval != 500*time.Millisecond || c.TaskRetryDelay != 30*time.Second ||
		c.TaskDeadline != 10*time.Minute || c.ProbeShutdownTimeout != 5*time.Second ||
		c.ReadyzTimeout != 2*time.Second {
		t.Fatalf("runtime defaults: %+v", c)
	}
}

func TestLoadInvalidAddr(t *testing.T) {
	_, err := Load(lookupFrom(map[string]string{"HTTP_ADDR": "abc"}))
	if err == nil || !strings.Contains(err.Error(), "HTTP_ADDR") {
		t.Fatalf("expected HTTP_ADDR error, got %v", err)
	}
	if strings.Contains(err.Error(), "abc") {
		t.Fatalf("error must not echo raw value: %v", err)
	}
}

func TestLoadInvalidLogLevel(t *testing.T) {
	_, err := Load(lookupFrom(map[string]string{"LOG_LEVEL": "verbose"}))
	if err == nil || !strings.Contains(err.Error(), "LOG_LEVEL") {
		t.Fatalf("expected LOG_LEVEL error, got %v", err)
	}
}

func TestLoadShutdownTooLong(t *testing.T) {
	_, err := Load(lookupFrom(map[string]string{"HTTP_SHUTDOWN_TIMEOUT": "60s"}))
	if err == nil || !strings.Contains(err.Error(), "HTTP_SHUTDOWN_TIMEOUT") {
		t.Fatalf("expected HTTP_SHUTDOWN_TIMEOUT error, got %v", err)
	}
}

// B0.2：DATABASE_URL 缺失时 Load 成功、ValidateRuntime 失败且只含变量名。
func TestValidateRuntimeMissing(t *testing.T) {
	c, err := Load(lookupFrom(nil))
	if err != nil {
		t.Fatalf("load without DATABASE_URL should succeed: %v", err)
	}
	err = c.ValidateRuntime()
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("expected DATABASE_URL missing error, got %v", err)
	}
	if strings.Contains(err.Error(), "://") {
		t.Fatalf("error must not echo DSN: %v", err)
	}
}

// B0.2：mysql:// 等 scheme 被 ValidateRuntime 拒绝。
func TestValidateRuntimeRejectsNonPostgres(t *testing.T) {
	c, _ := Load(lookupFrom(map[string]string{"DATABASE_URL": "mysql://u:p@127.0.0.1/db"}))
	err := c.ValidateRuntime()
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("expected invalid url error, got %v", err)
	}
	if strings.Contains(err.Error(), "mysql") {
		t.Fatalf("error must not echo scheme: %v", err)
	}
}

// B0.2：非法 bool / int / duration。
func TestLoadInvalidParsers(t *testing.T) {
	cases := []struct {
		vars map[string]string
		want string
	}{
		{map[string]string{"MIGRATE_ON_START": "yes"}, "MIGRATE_ON_START"},
		{map[string]string{"OUTBOX_BATCH": "abc"}, "OUTBOX_BATCH"},
		{map[string]string{"TASK_LEASE": "soon"}, "TASK_LEASE"},
	}
	for _, c := range cases {
		_, err := Load(lookupFrom(c.vars))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Fatalf("expected %s error for %v, got %v", c.want, c.vars, err)
		}
	}
}

// B0.2：边界值校验。
func TestLoadBoundaries(t *testing.T) {
	cases := []struct {
		vars map[string]string
		want string
	}{
		{map[string]string{"OUTBOX_BATCH": "0"}, "OUTBOX_BATCH"},
		{map[string]string{"OUTBOX_BATCH": "257"}, "OUTBOX_BATCH"},
		{map[string]string{"PROBE_WORKERS": "0"}, "PROBE_WORKERS"},
		{map[string]string{"PROBE_WORKERS": "65"}, "PROBE_WORKERS"},
		{map[string]string{"DB_MIN_CONNS": "11"}, "DB_MIN_CONNS"},
		{map[string]string{"TASK_MAX_ATTEMPTS": "11"}, "TASK_MAX_ATTEMPTS"},
	}
	for _, c := range cases {
		_, err := Load(lookupFrom(c.vars))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Fatalf("expected %s boundary error for %v, got %v", c.want, c.vars, err)
		}
	}
}
