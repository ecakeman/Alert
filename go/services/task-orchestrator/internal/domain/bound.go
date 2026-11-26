package domain

// InputBound 固定一个诊断任务可读的输入范围。任务创建后，新输入不得扩大该范围。
type InputBound struct {
	IncidentID       string
	From             int64
	To               int64
	MonitorWatermark int64
	AppWatermark     int64
}

// ReadReason 描述读取被拒绝的原因。
type ReadReason string

const (
	ReasonInvalidRange  ReadReason = "INVALID_RANGE"
	ReasonCrossIncident ReadReason = "CROSS_INCIDENT"
	ReasonOutOfRange    ReadReason = "OUT_OF_RANGE"
)

// ValidateBound 校验范围自身合法。
func ValidateBound(b InputBound) error {
	if b.From <= 0 || b.From > b.To {
		return newError(CodeInvalidRange, "invalid input range")
	}
	return nil
}

// CanRead 判断请求的版本/水位线是否落在任务绑定范围内；越界一律拒绝。
func CanRead(b InputBound, incidentID string, requestedVersion, requestedWatermark int64) (bool, ReadReason) {
	if b.From <= 0 || b.From > b.To {
		return false, ReasonInvalidRange
	}
	if incidentID != b.IncidentID {
		return false, ReasonCrossIncident
	}
	if requestedVersion < b.From || requestedVersion > b.To {
		return false, ReasonOutOfRange
	}
	if requestedWatermark > b.MonitorWatermark || requestedWatermark > b.AppWatermark {
		return false, ReasonOutOfRange
	}
	return true, ""
}