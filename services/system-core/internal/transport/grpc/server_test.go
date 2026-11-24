package grpc

import (
	"context"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/ecakeman/alert/services/system-core/internal/application/apperr"
	apptask "github.com/ecakeman/alert/services/system-core/internal/application/task"
	"github.com/ecakeman/alert/services/system-core/internal/domain"
	systemv1 "github.com/ecakeman/alert/services/system-core/internal/transport/grpc/gen/alertflow/system/v1"
)

func TestGRPCErrorMapping(t *testing.T) {
	cases := []struct {
		code string
		want codes.Code
	}{
		{apperr.CodeNotFound, codes.NotFound},
		{domain.CodeStateConflict, codes.FailedPrecondition},
		{domain.CodeStaleGeneration, codes.FailedPrecondition},
		{domain.CodeLeaseLost, codes.FailedPrecondition},
		{domain.CodeTypeMismatch, codes.FailedPrecondition},
		{domain.CodeServiceBusy, codes.Aborted},
		{domain.CodeOutOfRange, codes.InvalidArgument},
		{domain.CodeUnknownArtifact, codes.InvalidArgument},
		{apperr.CodeDependencyUnavailable, codes.Unavailable},
		{"SOMETHING_ELSE", codes.Internal},
	}
	for _, tc := range cases {
		err := mapError(apperr.New(tc.code, "detail"))
		if status.Code(err) != tc.want {
			t.Fatalf("code %s: want %s, got %s", tc.code, tc.want, status.Code(err))
		}
		if !strings.HasPrefix(status.Convert(err).Message(), tc.code+":") {
			t.Fatalf("code %s: message must keep stable prefix, got %q", tc.code, status.Convert(err).Message())
		}
	}
}

func TestServerRejectsMissingIdentity(t *testing.T) {
	server := NewServer(&apptask.Service{})
	_, err := server.ReportTaskStarted(context.Background(), &systemv1.ReportTaskStartedRequest{})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("missing identity should be InvalidArgument, got %s", status.Code(err))
	}
	if !strings.HasPrefix(status.Convert(err).Message(), apperr.CodeValidation+":") {
		t.Fatalf("message must keep stable prefix, got %q", status.Convert(err).Message())
	}
}
