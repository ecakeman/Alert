package domain

// Error 是领域规则返回的稳定错误，Code 用于跨层映射，Message 面向人。
type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// 稳定错误码。新增时集中在此，禁止把领域拒绝伪装成 INTERNAL。
const (
	CodeInvalidTransition = "INVALID_TRANSITION"
	CodeStaleGeneration   = "STALE_GENERATION"
	CodeLeaseLost         = "LEASE_LOST"
	CodeStateConflict     = "STATE_CONFLICT"
	CodeTypeMismatch      = "TYPE_MISMATCH"
	CodeNotFound          = "NOT_FOUND"
	CodeServiceBusy       = "SERVICE_BUSY"
	CodeOutOfRange        = "OUT_OF_RANGE"
	CodeCrossIncident     = "CROSS_INCIDENT"
	CodeInvalidRange      = "INVALID_RANGE"
	CodeEventIDConflict   = "EVENT_ID_CONFLICT"
	CodeNotResolved       = "NOT_RESOLVED"
	CodeArtifactMissing   = "ARTIFACT_MISSING"
	CodeHashMismatch      = "HASH_MISMATCH"
	CodeOwnerMismatch     = "OWNER_MISMATCH"
	CodeUnknownArtifact   = "UNKNOWN_ARTIFACT_TYPE"
)

func newError(code, message string) error { return &Error{Code: code, Message: message} }

// IsCode 判断错误是否为指定稳定 Code。
func IsCode(err error, code string) bool {
	if err == nil {
		return false
	}
	e, ok := err.(*Error)
	return ok && e.Code == code
}