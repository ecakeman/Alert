package contracts

import (
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestDispatchSnapshotJSONFields(t *testing.T) {
	snapshot := DispatchSnapshot{
		EpisodeID:             "22222222-2222-2222-2222-222222222222",
		ServiceID:             "47ceb5ca-3235-43c0-9089-53d7bdf412ed",
		EpisodeStatus:         "ACTIVE",
		InputVersionFrom:      1,
		InputVersionTo:        3,
		MonitorWatermark:      42,
		AppEventWatermark:     7,
		FinalInputVersion:     3,
		ProcessedInputVersion: 1,
		Reason:                "INPUT_APPENDED",
		ArtifactReadiness: ArtifactReadiness{
			Investigation:   true,
			DiagnosisReport: true,
			HashesMatch:     true,
		},
	}

	out, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("序列化 DispatchSnapshot 失败: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(out, &fields); err != nil {
		t.Fatalf("解析 DispatchSnapshot 失败: %v", err)
	}

	want := []string{
		"episode_id", "service_id", "episode_status",
		"input_version_from", "input_version_to",
		"monitor_watermark", "app_event_watermark",
		"final_input_version", "processed_input_version",
		"reason", "artifact_readiness",
	}
	for _, key := range want {
		if _, ok := fields[key]; !ok {
			t.Errorf("DispatchSnapshot 缺少字段 %q", key)
		}
	}
	if len(fields) != len(want) {
		t.Errorf("DispatchSnapshot 字段数 = %d，期望 %d", len(fields), len(want))
	}

	var readiness map[string]json.RawMessage
	if err := json.Unmarshal(fields["artifact_readiness"], &readiness); err != nil {
		t.Fatalf("解析 artifact_readiness 失败: %v", err)
	}
	for _, key := range []string{"investigation", "diagnosis_report", "hashes_match"} {
		if _, ok := readiness[key]; !ok {
			t.Errorf("artifact_readiness 缺少字段 %q", key)
		}
	}
	if len(readiness) != 3 {
		t.Errorf("artifact_readiness 字段数 = %d，期望 3", len(readiness))
	}
}

// jsonFieldNames 读取 struct tag 中的 json 名（忽略 omitempty），
// 避免零值 + omitempty 在序列化时隐藏字段导致误判。
func jsonFieldNames(value any) map[string]bool {
	typ := reflect.TypeOf(value)
	out := make(map[string]bool, typ.NumField())
	for i := 0; i < typ.NumField(); i++ {
		tag := typ.Field(i).Tag.Get("json")
		name := strings.Split(tag, ",")[0]
		if name == "" || name == "-" {
			continue
		}
		out[name] = true
	}
	return out
}

func TestDTOJSONTagsMatchLegacyShape(t *testing.T) {
	cases := []struct {
		name   string
		value  any
		fields []string
	}{
		{
			name:  "ServiceView",
			value: ServiceView{},
			fields: []string{
				"service_id", "service_key", "name", "description", "enabled",
				"monitor_aggregation_mode", "authority_producer_id", "monitor_status",
				"app_status", "targets", "target_count", "last_check_at", "active_episode_id",
			},
		},
		{
			name:  "Target",
			value: Target{},
			fields: []string{
				"target_id", "target_key", "target_name", "target_type", "address", "config",
				"interval_ms", "timeout_ms", "failure_threshold", "recovery_threshold",
				"is_required", "enabled", "status", "consecutive_failures",
				"consecutive_successes", "last_check_at", "config_generation",
			},
		},
		{
			name:  "MonitorPoint",
			value: MonitorPoint{},
			fields: []string{
				"point_seq", "target_id", "target_key", "target_type", "observed_at",
				"check_status", "status_class", "error_class",
			},
		},
		{
			name:  "AppEventInput",
			value: AppEventInput{},
			fields: []string{
				"producer_id", "event_id", "event_kind", "health_status", "payload", "event_time",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			names := jsonFieldNames(tc.value)
			for _, key := range tc.fields {
				if !names[key] {
					t.Errorf("%s 缺少 JSON tag %q", tc.name, key)
				}
			}
		})
	}
}

func TestNewTraceID(t *testing.T) {
	id := NewTraceID()
	if len(id) != TraceIDLength {
		t.Fatalf("trace id 长度 = %d，期望 %d", len(id), TraceIDLength)
	}
	if _, err := hex.DecodeString(id); err != nil {
		t.Fatalf("trace id 不是十六进制: %v", err)
	}
	if second := NewTraceID(); second == id {
		t.Fatal("连续生成的两个 trace id 相同")
	}
}
