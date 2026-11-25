package domain

// SourceStatus 是单来源（MONITOR / APP）的状态三值。
type SourceStatus string

const (
	SourceUnknown  SourceStatus = "UNKNOWN"
	SourceNormal   SourceStatus = "NORMAL"
	SourceAbnormal SourceStatus = "ABNORMAL"
)

// Source 标识输入来源。
type Source string

const (
	SourceMonitor Source = "MONITOR"
	SourceApp     Source = "APP"
)

// ParseSourceStatus 解析外部输入状态；缺省、空值、非法值一律映射为 UNKNOWN，绝不隐式转 NORMAL。
func ParseSourceStatus(raw string) SourceStatus {
	switch raw {
	case "NORMAL":
		return SourceNormal
	case "ABNORMAL":
		return SourceAbnormal
	default:
		return SourceUnknown
	}
}

// IsNormal 仅当状态为 NORMAL 时为真；UNKNOWN 与 ABNORMAL 均为假。
func IsNormal(s SourceStatus) bool { return s == SourceNormal }
