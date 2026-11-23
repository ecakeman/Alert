package apperr

// Error 是应用层稳定业务错误；不依赖 domain / gin / pgx。
type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// 应用层稳定错误码。
const (
	CodeValidation            = "VALIDATION"
	CodeNotFound              = "NOT_FOUND"
	CodeAlreadyExists         = "ALREADY_EXISTS"
	CodeNotImplemented        = "NOT_IMPLEMENTED"
	CodeDependencyUnavailable = "DEPENDENCY_UNAVAILABLE"
	CodeArtifactNotFound      = "ARTIFACT_NOT_FOUND"
)

func New(code, message string) *Error { return &Error{Code: code, Message: message} }

func Validation(message string) *Error            { return New(CodeValidation, message) }
func NotFound(message string) *Error              { return New(CodeNotFound, message) }
func AlreadyExists(message string) *Error         { return New(CodeAlreadyExists, message) }
func NotImplemented(message string) *Error        { return New(CodeNotImplemented, message) }
func DependencyUnavailable(message string) *Error { return New(CodeDependencyUnavailable, message) }
func ArtifactNotFound(message string) *Error      { return New(CodeArtifactNotFound, message) }