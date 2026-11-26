package logging

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestNewRejectsInvalidLevelWithoutEchoingValue(t *testing.T) {
	_, err := New(Config{Service: "svc", Level: "s3cret-level"})
	if err == nil {
		t.Fatal("非法 level 期望报错")
	}
	var le *LoadError
	if !errors.As(err, &le) {
		t.Fatalf("期望 *LoadError，实际 %T", err)
	}
	if le.Var != "LOG_LEVEL" || le.Reason != "unsupported value" {
		t.Fatalf("LoadError = %+v", le)
	}
	if strings.Contains(err.Error(), "s3cret-level") {
		t.Fatal("错误文本回显了原值")
	}
}

func TestNewRequiresServiceName(t *testing.T) {
	_, err := New(Config{Service: " "})
	var le *LoadError
	if !errors.As(err, &le) || le.Var != "SERVICE_NAME" {
		t.Fatalf("期望 SERVICE_NAME LoadError，实际 %v", err)
	}
}

func TestNewWritesStructuredFields(t *testing.T) {
	var buf bytes.Buffer
	logger, err := New(Config{Service: "monitor", Level: "debug", JSON: true, Output: &buf})
	if err != nil {
		t.Fatalf("New 失败: %v", err)
	}
	WithTrace(logger, "0123456789abcdef0123456789abcdef").Info("probe started", "event_id", "evt-1")

	out := buf.String()
	for _, want := range []string{`"service":"monitor"`, `"trace_id":"0123456789abcdef0123456789abcdef"`, `"event_id":"evt-1"`, `"level":"INFO"`} {
		if !strings.Contains(out, want) {
			t.Errorf("日志缺少 %s，实际 %s", want, out)
		}
	}
}

func TestWithTraceAndEventNilSafe(t *testing.T) {
	if WithTrace(nil, "x") != nil {
		t.Fatal("nil logger 应保持 nil")
	}
	var buf bytes.Buffer
	logger, _ := New(Config{Service: "svc", Output: &buf})
	if got := WithEvent(logger, ""); got == nil {
		t.Fatal("空 event_id 应返回原 logger")
	}
}
