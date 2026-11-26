package config

import (
	"errors"
	"strings"
	"testing"
)

func env(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestLoadRequiresDatabaseURL(t *testing.T) {
	_, err := Load(env(nil))
	var le *LoadError
	if !errors.As(err, &le) || le.Var != "DATABASE_URL" {
		t.Fatalf("期望 DATABASE_URL LoadError，实际 %v", err)
	}
}

func TestLoadRejectsInvalidDatabaseURL(t *testing.T) {
	_, err := Load(env(map[string]string{"DATABASE_URL": "mysql://u:p@h/db"}))
	var le *LoadError
	if !errors.As(err, &le) || le.Var != "DATABASE_URL" {
		t.Fatalf("期望 DATABASE_URL LoadError，实际 %v", err)
	}
	if strings.Contains(err.Error(), "p@h") {
		t.Fatal("错误文本泄露了 DATABASE_URL 值")
	}
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load(env(map[string]string{"DATABASE_URL": "postgres://alert_registry:pw@127.0.0.1:5432/alert?sslmode=disable"}))
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}
	if cfg.HTTPAddr != ":8081" || cfg.KafkaPublishTopic != "alertflow.registry.events.v1" {
		t.Fatalf("默认值错误: %+v", cfg)
	}
	if len(cfg.KafkaConsumeTopics) != 1 || cfg.KafkaConsumeTopics[0] != "alertflow.monitor.events.v1" {
		t.Fatalf("默认消费 topic 错误: %v", cfg.KafkaConsumeTopics)
	}
	if cfg.DBConfig().MaxConns != 10 {
		t.Fatalf("默认 DB_MAX_CONNS 错误: %+v", cfg.DBConfig())
	}
}
