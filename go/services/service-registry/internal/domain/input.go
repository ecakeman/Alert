package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
)

// MonitorField 是参与输入指纹的稳定字段白名单（对齐基线 system.hash.go）。
// 采样时间、接收时间、噪声字段不在其中，因此不进入指纹。
type MonitorField struct {
	TargetKey        string `json:"target_key"`
	TargetType       string `json:"target_type"`
	Status           string `json:"status"`
	StatusClass      string `json:"status_class"`
	ErrorClass       string `json:"error_class"`
	StateFingerprint string `json:"state_fingerprint,omitempty"`
}

// AppCanon 参与指纹的应用事件字段；event_id 与时间不在其中。
type AppCanon struct {
	EventKind    string          `json:"event_kind"`
	HealthStatus string          `json:"health_status"`
	Payload      json.RawMessage `json:"payload"`
}

type canonicalInput struct {
	AppEvents         []AppCanon     `json:"app_events"`
	MonitorProjection []MonitorField `json:"monitor_projection"`
}

// Fingerprint 计算输入指纹：投影按 target_key 排序，应用事件保持追加顺序。
func Fingerprint(projection []MonitorField, events []AppCanon) (string, error) {
	proj := append([]MonitorField(nil), projection...)
	sort.Slice(proj, func(i, j int) bool { return proj[i].TargetKey < proj[j].TargetKey })
	if events == nil {
		events = []AppCanon{}
	}
	body, err := json.Marshal(canonicalInput{AppEvents: events, MonitorProjection: proj})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), nil
}

// AppEvent 是应用侧事件的规范化形态。身份键为 (ServiceID, ProducerID, EventID)。
type AppEvent struct {
	ServiceID    string
	ProducerID   string
	EventID      string
	EventKind    string
	HealthStatus string
	Payload      json.RawMessage
	ContentHash  string
}

// AppEventDecision 是应用事件幂等判定结果。
type AppEventDecision int

const (
	AppEventAppend AppEventDecision = iota
	AppEventDuplicate
	AppEventConflict
)

type AppEventResult struct {
	Decision AppEventDecision
	Err      error
}

func sameIdentity(a, b AppEvent) bool {
	return a.ServiceID == b.ServiceID && a.ProducerID == b.ProducerID && a.EventID == b.EventID
}

// SameAppEvent 判断内容是否一致（不含身份键；时间不参与）。
func SameAppEvent(a, b AppEvent) bool {
	if a.ContentHash != "" && b.ContentHash != "" {
		return a.ContentHash == b.ContentHash
	}
	return a.EventKind == b.EventKind &&
		a.HealthStatus == b.HealthStatus &&
		string(a.Payload) == string(b.Payload)
}

// DecideAppEvent 判定：新键追加；同键同内容幂等；同键不同内容冲突且不覆盖。
func DecideAppEvent(existing *AppEvent, incoming AppEvent) AppEventResult {
	if existing == nil || !sameIdentity(*existing, incoming) {
		return AppEventResult{Decision: AppEventAppend}
	}
	if SameAppEvent(*existing, incoming) {
		return AppEventResult{Decision: AppEventDuplicate}
	}
	return AppEventResult{
		Decision: AppEventConflict,
		Err:      newError(CodeEventIDConflict, "app event identity reused with different content"),
	}
}
