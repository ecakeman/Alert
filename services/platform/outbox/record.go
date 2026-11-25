package outbox

import "time"

// Record 是 Outbox 存储行：业务信封字段 + 投递控制字段。
// 与 contracts.Event 解耦：platform 不 import contracts，由服务层在边界转换。
type Record struct {
	EventID         string
	EventType       string
	SchemaVersion   int
	AggregateType   string
	AggregateID     string
	OccurredAt      time.Time
	TraceID         string
	Payload         []byte
	DedupeKey       *string
	PublishedAt     *time.Time
	PublishAttempts int
	NextRetryAt     *time.Time
	ClaimOwner      *string
	ClaimUntil      *time.Time
	LastError       string
}
