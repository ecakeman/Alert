package config

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func env(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load(env(nil))
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}
	if cfg.ServiceName != "console-bff" || cfg.HTTPAddr != ":8080" {
		t.Fatalf("默认值错误: %+v", cfg)
	}
	if cfg.UpstreamTimeout != 3*time.Second || cfg.AggregateTimeout != 800*time.Millisecond {
		t.Fatalf("默认超时错误: %+v", cfg)
	}
	if cfg.RegistryBaseURL != "http://127.0.0.1:8081" || cfg.TaskBaseURL != "http://127.0.0.1:8081" {
		t.Fatalf("默认下游地址错误: %+v", cfg)
	}
}

func TestLoadRejectsDatabaseURL(t *testing.T) {
	_, err := Load(env(map[string]string{"DATABASE_URL": "postgres://u:s3cret@127.0.0.1:5432/alert"}))
	var le *LoadError
	if !errors.As(err, &le) || le.Var != "DATABASE_URL" {
		t.Fatalf("期望 DATABASE_URL LoadError，实际 %v", err)
	}
	if strings.Contains(err.Error(), "s3cret") {
		t.Fatal("错误文本泄露了 DATABASE_URL 值")
	}
}

func TestLoadRejectsInvalidBaseURL(t *testing.T) {
	_, err := Load(env(map[string]string{"REGISTRY_BASE_URL": "127.0.0.1:8081"}))
	var le *LoadError
	if !errors.As(err, &le) || le.Var != "REGISTRY_BASE_URL" {
		t.Fatalf("期望 REGISTRY_BASE_URL LoadError，实际 %v", err)
	}
}

func TestLoadRejectsBadDuration(t *testing.T) {
	_, err := Load(env(map[string]string{"UPSTREAM_TIMEOUT": "-1s"}))
	var le *LoadError
	if !errors.As(err, &le) || le.Var != "UPSTREAM_TIMEOUT" {
		t.Fatalf("期望 UPSTREAM_TIMEOUT LoadError，实际 %v", err)
	}
}