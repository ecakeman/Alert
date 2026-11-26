package grpc

import (
	"context"

	googlegrpc "google.golang.org/grpc"

	"github.com/ecakeman/alert/services/task-orchestrator/internal/application/apperr"
	apptask "github.com/ecakeman/alert/services/task-orchestrator/internal/application/task"
	systemv1 "github.com/ecakeman/alert/services/task-orchestrator/internal/transport/grpc/gen/alertflow/system/v1"
)

// Server 实现 7 类回调 RPC，错误统一经 errcode.go 映射。
type Server struct {
	systemv1.UnimplementedSystemTaskGatewayServiceServer
	tasks *apptask.Service
}

func NewServer(tasks *apptask.Service) *Server { return &Server{tasks: tasks} }

func Register(reg googlegrpc.ServiceRegistrar, srv *Server) {
	systemv1.RegisterSystemTaskGatewayServiceServer(reg, srv)
}

func (s *Server) ReportTaskStarted(ctx context.Context, req *systemv1.ReportTaskStartedRequest) (*systemv1.TaskAck, error) {
	if req == nil || req.Identity == nil {
		return nil, mapError(apperr.Validation("missing task identity"))
	}
	if err := s.tasks.Started(ctx, toTaskIdentity(req.Identity)); err != nil {
		return nil, mapError(err)
	}
	return &systemv1.TaskAck{Accepted: true}, nil
}

func (s *Server) ReportTaskHeartbeat(ctx context.Context, req *systemv1.ReportTaskHeartbeatRequest) (*systemv1.TaskAck, error) {
	if req == nil || req.Identity == nil {
		return nil, mapError(apperr.Validation("missing task identity"))
	}
	if err := s.tasks.Heartbeat(ctx, toTaskIdentity(req.Identity)); err != nil {
		return nil, mapError(err)
	}
	return &systemv1.TaskAck{Accepted: true}, nil
}

func (s *Server) RegisterTaskArtifact(ctx context.Context, req *systemv1.RegisterTaskArtifactRequest) (*systemv1.TaskAck, error) {
	if req == nil || req.Identity == nil {
		return nil, mapError(apperr.Validation("missing task identity"))
	}
	if err := s.tasks.RegisterArtifact(ctx, toRegisterArtifactInput(req)); err != nil {
		return nil, mapError(err)
	}
	return &systemv1.TaskAck{Accepted: true}, nil
}

func (s *Server) ReportTaskResult(ctx context.Context, req *systemv1.ReportTaskResultRequest) (*systemv1.TaskAck, error) {
	if req == nil || req.Identity == nil {
		return nil, mapError(apperr.Validation("missing task identity"))
	}
	if err := s.tasks.Complete(ctx, toTaskIdentity(req.Identity)); err != nil {
		return nil, mapError(err)
	}
	return &systemv1.TaskAck{Accepted: true}, nil
}

func (s *Server) ReportTaskFailure(ctx context.Context, req *systemv1.ReportTaskFailureRequest) (*systemv1.TaskAck, error) {
	if req == nil || req.Identity == nil {
		return nil, mapError(apperr.Validation("missing task identity"))
	}
	class := apptask.FailureFatal
	if req.Retryable {
		class = apptask.FailureRetryable
	}
	if err := s.tasks.ReportFailure(ctx, toTaskIdentity(req.Identity), class, req.ErrorCode, req.ErrorMessage); err != nil {
		return nil, mapError(err)
	}
	return &systemv1.TaskAck{Accepted: true}, nil
}

func (s *Server) GetTaskContext(ctx context.Context, req *systemv1.GetTaskContextRequest) (*systemv1.GetTaskContextResponse, error) {
	if req == nil || req.Identity == nil {
		return nil, mapError(apperr.Validation("missing task identity"))
	}
	view, err := s.tasks.GetContext(ctx, toTaskIdentity(req.Identity), req.FromInputVersion, req.ToInputVersion, req.Watermark)
	if err != nil {
		return nil, mapError(err)
	}
	return fromTaskContext(view), nil
}

func (s *Server) AbandonTask(ctx context.Context, req *systemv1.AbandonTaskRequest) (*systemv1.TaskAck, error) {
	if req == nil || req.Identity == nil {
		return nil, mapError(apperr.Validation("missing task identity"))
	}
	if err := s.tasks.Abandon(ctx, toTaskIdentity(req.Identity)); err != nil {
		return nil, mapError(err)
	}
	return &systemv1.TaskAck{Accepted: true}, nil
}
