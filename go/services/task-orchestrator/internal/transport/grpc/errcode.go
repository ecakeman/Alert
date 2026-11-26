package grpc

import (
	"errors"

	googlegrpc "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/ecakeman/alert/services/task-orchestrator/internal/application/apperr"
	"github.com/ecakeman/alert/services/task-orchestrator/internal/domain"
)

// mapError 是稳定码 → gRPC code 的唯一映射点；message 前缀保留稳定码。
func mapError(err error) error {
	if err == nil {
		return nil
	}
	normalized := apperr.Normalize(err)
	var ae *apperr.Error
	if errors.As(normalized, &ae) {
		return status.Error(grpcCode(ae.Code), ae.Code+": "+ae.Message)
	}
	return status.Error(codes.Internal, "INTERNAL: internal error")
}

func grpcCode(code string) codes.Code {
	switch code {
	case apperr.CodeNotFound, domain.CodeArtifactMissing:
		return codes.NotFound
	case domain.CodeStateConflict, domain.CodeTypeMismatch, domain.CodeStaleGeneration,
		domain.CodeLeaseLost, domain.CodeInvalidTransition, domain.CodeHashMismatch,
		domain.CodeNotResolved, domain.CodeOwnerMismatch:
		return codes.FailedPrecondition
	case domain.CodeServiceBusy:
		return codes.Aborted
	case domain.CodeOutOfRange, domain.CodeCrossIncident, domain.CodeInvalidRange,
		domain.CodeUnknownArtifact, apperr.CodeValidation:
		return codes.InvalidArgument
	case apperr.CodeDependencyUnavailable:
		return codes.Unavailable
	default:
		return codes.Internal
	}
}

var _ = googlegrpc.SupportPackageIsVersion9
