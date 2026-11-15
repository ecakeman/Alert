package api

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/ecakeman/alertflow/services/system-core/internal/system"
	"github.com/google/uuid"
)

// ArtifactSource 由 Python gRPC 实现。为空时产物全文接口返回 503。
type ArtifactSource interface {
	GetArtifact(r *http.Request, episodeID uuid.UUID, artifactType string) (any, error)
}

// Server 保留现有 JSON 行为，供内部 gRPC 适配和测试调用。
type Server struct {
	Store     *system.Store
	Artifacts ArtifactSource
	Log       *slog.Logger
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /readyz", s.readyz)
	mux.HandleFunc("GET /api/v1/dashboard/summary", s.dashboard)
	mux.HandleFunc("GET /api/v1/services", s.listServices)
	mux.HandleFunc("POST /api/v1/services", s.createService)
	mux.HandleFunc("GET /api/v1/services/{service_id}", s.getService)
	mux.HandleFunc("PATCH /api/v1/services/{service_id}", s.patchService)
	mux.HandleFunc("DELETE /api/v1/services/{service_id}", s.deleteService)
	mux.HandleFunc("GET /api/v1/services/{service_id}/targets", s.listTargets)
	mux.HandleFunc("POST /api/v1/services/{service_id}/targets", s.createTarget)
	mux.HandleFunc("PATCH /api/v1/targets/{target_id}", s.patchTarget)
	mux.HandleFunc("DELETE /api/v1/targets/{target_id}", s.deleteTarget)
	mux.HandleFunc("GET /api/v1/episodes", s.listEpisodes)
	mux.HandleFunc("GET /api/v1/services/{service_id}/timeline", s.serviceTimeline)
	mux.HandleFunc("POST /api/v1/services/{service_id}/events", s.postEvent)
	mux.HandleFunc("GET /api/v1/episodes/{episode_id}", s.getEpisode)
	mux.HandleFunc("GET /api/v1/episodes/{episode_id}/timeline", s.episodeTimeline)
	mux.HandleFunc("POST /api/v1/episodes/{episode_id}/retry-diagnosis", s.retryDiagnosis)
	mux.HandleFunc("POST /api/v1/episodes/{episode_id}/retry-archive", s.retryArchive)
	return s.wrap(mux)
}

func (s *Server) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = uuid.NewString()
		}
		w.Header().Set("X-Request-ID", id)
		sw := &statusWriter{ResponseWriter: w, code: http.StatusOK}
		start := time.Now()
		defer func() {
			rec := recover()
			if rec != nil && !sw.wrote {
				if s.Log != nil {
					s.Log.Error("http panic", "request_id", id, "err", rec)
				}
				writeJSON(sw, http.StatusInternalServerError, map[string]string{
					"error_code": "INTERNAL", "message": "internal error",
				})
			}
			if s.Log != nil {
				s.Log.Info("http", "method", r.Method, "path", r.URL.Path, "status", sw.code, "request_id", id, "duration_ms", time.Since(start).Milliseconds())
			}
		}()
		next.ServeHTTP(sw, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	code  int
	wrote bool
}

func (w *statusWriter) WriteHeader(code int) {
	w.code = code
	w.wrote = true
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(p []byte) (int, error) {
	w.wrote = true
	return w.ResponseWriter.Write(p)
}

func (s *Server) createService(w http.ResponseWriter, r *http.Request) {
	var in system.CreateServiceInput
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	view, err := s.Store.CreateService(r.Context(), in)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, view)
}

func (s *Server) getService(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("service_id"))
	if err != nil {
		writeJSON(w, 400, map[string]string{"error_code": "VALIDATION", "message": "invalid service_id"})
		return
	}
	view, err := s.Store.GetService(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) patchService(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("service_id"))
	if err != nil {
		writeJSON(w, 400, map[string]string{"error_code": "VALIDATION", "message": "invalid service_id"})
		return
	}
	var in system.PatchServiceInput
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	view, err := s.Store.PatchService(r.Context(), id, in)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) serviceTimeline(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("service_id"))
	if err != nil {
		writeJSON(w, 400, map[string]string{"error_code": "VALIDATION", "message": "invalid service_id"})
		return
	}
	records, err := s.Store.ServiceTimeline(r.Context(), id, 100)
	if err != nil {
		writeErr(w, err)
		return
	}
	if records == nil {
		records = []system.TimelineRecord{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"records": records})
}

func (s *Server) postEvent(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("service_id"))
	if err != nil {
		writeJSON(w, 400, map[string]string{"error_code": "VALIDATION", "message": "invalid service_id"})
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeJSON(w, 400, map[string]string{"error_code": "VALIDATION", "message": "cannot read body"})
		return
	}
	var ev system.AppEvent
	if err := json.Unmarshal(raw, &ev); err != nil {
		writeJSON(w, 400, map[string]string{"error_code": "VALIDATION", "message": "invalid json"})
		return
	}
	res, err := s.Store.ApplyAppEvent(r.Context(), id, ev, raw)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, res.Status, res.Body)
}

func (s *Server) getEpisode(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("episode_id"))
	if err != nil {
		writeJSON(w, 400, map[string]string{"error_code": "VALIDATION", "message": "invalid episode_id"})
		return
	}
	view, err := s.Store.GetEpisode(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) episodeTimeline(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("episode_id"))
	if err != nil {
		writeJSON(w, 400, map[string]string{"error_code": "VALIDATION", "message": "invalid episode_id"})
		return
	}
	body, err := s.Store.EpisodeTimeline(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, body)
}

func (s *Server) retryDiagnosis(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("episode_id"))
	if err != nil {
		writeJSON(w, 400, map[string]string{"error_code": "VALIDATION", "message": "invalid episode_id"})
		return
	}
	taskID, err := s.Store.RetryDiagnosis(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"task_id": taskID})
}

func (s *Server) retryArchive(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("episode_id"))
	if err != nil {
		writeJSON(w, 400, map[string]string{"error_code": "VALIDATION", "message": "invalid episode_id"})
		return
	}
	taskID, err := s.Store.RetryArchive(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"task_id": taskID})
}

func decode(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return &system.Error{Code: "VALIDATION", Message: err.Error(), HTTP: 400}
	}
	return nil
}

func writeErr(w http.ResponseWriter, err error) {
	var se *system.Error
	if errors.As(err, &se) {
		writeJSON(w, se.HTTP, map[string]string{"error_code": se.Code, "message": se.Message})
		return
	}
	writeJSON(w, 500, map[string]string{"error_code": "INTERNAL", "message": err.Error()})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
