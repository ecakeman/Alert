package systemapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	systemv1 "github.com/ecakeman/alertflow/services/system-core/gen/system/v1"
	"github.com/ecakeman/alertflow/services/system-core/internal/api"
	"google.golang.org/grpc/health/grpc_health_v1"
)

// Server 把现有 JSON 行为暴露为内部 SystemApi。
type Server struct {
	systemv1.UnimplementedSystemApiServer
	grpc_health_v1.UnimplementedHealthServer
	API *api.Server
}

func (s *Server) Ready(ctx context.Context, _ *systemv1.ReadyRequest) (*systemv1.ReadyResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := s.API.Store.DB.PingContext(ctx); err != nil {
		return &systemv1.ReadyResponse{Status: "unavailable"}, nil
	}
	return &systemv1.ReadyResponse{Status: "ok"}, nil
}

func (s *Server) Check(ctx context.Context, _ *grpc_health_v1.HealthCheckRequest) (*grpc_health_v1.HealthCheckResponse, error) {
	ready, _ := s.Ready(ctx, nil)
	if ready.GetStatus() != "ok" {
		return &grpc_health_v1.HealthCheckResponse{Status: grpc_health_v1.HealthCheckResponse_NOT_SERVING}, nil
	}
	return &grpc_health_v1.HealthCheckResponse{Status: grpc_health_v1.HealthCheckResponse_SERVING}, nil
}

func (s *Server) Watch(_ *grpc_health_v1.HealthCheckRequest, stream grpc_health_v1.Health_WatchServer) error {
	resp, err := s.Check(stream.Context(), nil)
	if err != nil {
		return err
	}
	return stream.Send(resp)
}

func (s *Server) Dashboard(ctx context.Context, in *systemv1.JsonCall) (*systemv1.JsonResult, error) {
	return s.dispatch(ctx, http.MethodGet, "/api/v1/dashboard/summary", in)
}

func (s *Server) ListServices(ctx context.Context, in *systemv1.JsonCall) (*systemv1.JsonResult, error) {
	return s.dispatch(ctx, http.MethodGet, "/api/v1/services", in)
}

func (s *Server) CreateService(ctx context.Context, in *systemv1.JsonCall) (*systemv1.JsonResult, error) {
	return s.dispatch(ctx, http.MethodPost, "/api/v1/services", in)
}

func (s *Server) GetService(ctx context.Context, in *systemv1.JsonCall) (*systemv1.JsonResult, error) {
	return s.dispatch(ctx, http.MethodGet, "/api/v1/services/"+in.GetId(), in)
}

func (s *Server) PatchService(ctx context.Context, in *systemv1.JsonCall) (*systemv1.JsonResult, error) {
	return s.dispatch(ctx, http.MethodPatch, "/api/v1/services/"+in.GetId(), in)
}

func (s *Server) DeleteService(ctx context.Context, in *systemv1.JsonCall) (*systemv1.JsonResult, error) {
	return s.dispatch(ctx, http.MethodDelete, "/api/v1/services/"+in.GetId(), in)
}

func (s *Server) ListTargets(ctx context.Context, in *systemv1.JsonCall) (*systemv1.JsonResult, error) {
	return s.dispatch(ctx, http.MethodGet, "/api/v1/services/"+in.GetId()+"/targets", in)
}

func (s *Server) CreateTarget(ctx context.Context, in *systemv1.JsonCall) (*systemv1.JsonResult, error) {
	return s.dispatch(ctx, http.MethodPost, "/api/v1/services/"+in.GetId()+"/targets", in)
}

func (s *Server) PatchTarget(ctx context.Context, in *systemv1.JsonCall) (*systemv1.JsonResult, error) {
	return s.dispatch(ctx, http.MethodPatch, "/api/v1/targets/"+in.GetId(), in)
}

func (s *Server) DeleteTarget(ctx context.Context, in *systemv1.JsonCall) (*systemv1.JsonResult, error) {
	return s.dispatch(ctx, http.MethodDelete, "/api/v1/targets/"+in.GetId(), in)
}

func (s *Server) ListEpisodes(ctx context.Context, in *systemv1.JsonCall) (*systemv1.JsonResult, error) {
	return s.dispatch(ctx, http.MethodGet, "/api/v1/episodes", in)
}

func (s *Server) ServiceTimeline(ctx context.Context, in *systemv1.JsonCall) (*systemv1.JsonResult, error) {
	return s.dispatch(ctx, http.MethodGet, "/api/v1/services/"+in.GetId()+"/timeline", in)
}

func (s *Server) PostEvent(ctx context.Context, in *systemv1.JsonCall) (*systemv1.JsonResult, error) {
	return s.dispatch(ctx, http.MethodPost, "/api/v1/services/"+in.GetId()+"/events", in)
}

func (s *Server) GetEpisode(ctx context.Context, in *systemv1.JsonCall) (*systemv1.JsonResult, error) {
	return s.dispatch(ctx, http.MethodGet, "/api/v1/episodes/"+in.GetId(), in)
}

func (s *Server) EpisodeTimeline(ctx context.Context, in *systemv1.JsonCall) (*systemv1.JsonResult, error) {
	return s.dispatch(ctx, http.MethodGet, "/api/v1/episodes/"+in.GetId()+"/timeline", in)
}

func (s *Server) RetryDiagnosis(ctx context.Context, in *systemv1.JsonCall) (*systemv1.JsonResult, error) {
	return s.dispatch(ctx, http.MethodPost, "/api/v1/episodes/"+in.GetId()+"/retry-diagnosis", in)
}

func (s *Server) RetryArchive(ctx context.Context, in *systemv1.JsonCall) (*systemv1.JsonResult, error) {
	return s.dispatch(ctx, http.MethodPost, "/api/v1/episodes/"+in.GetId()+"/retry-archive", in)
}

func (s *Server) dispatch(ctx context.Context, method, path string, in *systemv1.JsonCall) (*systemv1.JsonResult, error) {
	if in.GetQuery() != "" {
		path += "?" + strings.TrimPrefix(in.GetQuery(), "?")
	}
	req := httptest.NewRequest(method, path, strings.NewReader(in.GetBody())).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.API.Handler().ServeHTTP(rec, req)
	return &systemv1.JsonResult{HttpStatus: int32(rec.Code), Body: rec.Body.String()}, nil
}
