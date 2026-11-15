package api

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/ecakeman/alertflow/services/system-core/internal/system"
	"github.com/google/uuid"
)

func (s *Server) readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.Store.DB.PingContext(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable", "database": "down"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	summary, err := s.Store.Dashboard(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (s *Server) listServices(w http.ResponseWriter, r *http.Request) {
	limit, ok := queryInt(w, r, "limit")
	if !ok {
		return
	}
	offset, ok := queryInt(w, r, "offset")
	if !ok {
		return
	}
	services, total, err := s.Store.ListServices(r.Context(), system.ServiceFilter{
		Status: r.URL.Query().Get("status"),
		Q:      r.URL.Query().Get("q"),
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	if services == nil {
		services = []system.ServiceSummary{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"services": services, "total": total, "limit": pageLimit(limit, 50, 200), "offset": offset,
	})
}

func (s *Server) deleteService(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUIDParam(w, r, "service_id", "invalid service_id")
	if !ok {
		return
	}
	if err := s.Store.SoftDeleteService(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	s.Cache.Delete(r.Context(), "service:"+id.String())
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listTargets(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUIDParam(w, r, "service_id", "invalid service_id")
	if !ok {
		return
	}
	targets, err := s.Store.GetTargets(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	if targets == nil {
		targets = []system.Target{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"targets": targets})
}

func (s *Server) createTarget(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUIDParam(w, r, "service_id", "invalid service_id")
	if !ok {
		return
	}
	var in system.TargetInput
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	target, err := s.Store.CreateTarget(r.Context(), id, in)
	if err != nil {
		writeErr(w, err)
		return
	}
	s.Cache.Delete(r.Context(), "service:"+id.String())
	writeJSON(w, http.StatusCreated, target)
}

func (s *Server) patchTarget(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUIDParam(w, r, "target_id", "invalid target_id")
	if !ok {
		return
	}
	var in system.PatchTargetInput
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	target, serviceID, err := s.Store.UpdateTarget(r.Context(), id, in)
	if err != nil {
		writeErr(w, err)
		return
	}
	s.Cache.Delete(r.Context(), "service:"+serviceID.String())
	writeJSON(w, http.StatusOK, target)
}

func (s *Server) deleteTarget(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUIDParam(w, r, "target_id", "invalid target_id")
	if !ok {
		return
	}
	serviceID, err := s.Store.SoftDeleteTarget(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	s.Cache.Delete(r.Context(), "service:"+serviceID.String())
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listEpisodes(w http.ResponseWriter, r *http.Request) {
	limit, ok := queryInt(w, r, "limit")
	if !ok {
		return
	}
	offset, ok := queryInt(w, r, "offset")
	if !ok {
		return
	}
	filter := system.EpisodeFilter{Status: r.URL.Query().Get("status"), Limit: limit, Offset: offset}
	if raw := r.URL.Query().Get("service_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			writeJSON(w, 400, map[string]string{"error_code": "VALIDATION", "message": "invalid service_id"})
			return
		}
		filter.ServiceID = &id
	}
	if raw := r.URL.Query().Get("from"); raw != "" {
		ts, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeJSON(w, 400, map[string]string{"error_code": "VALIDATION", "message": "invalid from"})
			return
		}
		filter.From = &ts
	}
	if raw := r.URL.Query().Get("to"); raw != "" {
		ts, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeJSON(w, 400, map[string]string{"error_code": "VALIDATION", "message": "invalid to"})
			return
		}
		filter.To = &ts
	}
	episodes, total, err := s.Store.ListEpisodes(r.Context(), filter)
	if err != nil {
		writeErr(w, err)
		return
	}
	if episodes == nil {
		episodes = []system.EpisodeSummary{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"episodes": episodes, "total": total, "limit": pageLimit(limit, 20, 100), "offset": offset,
	})
}

func parseUUIDParam(w http.ResponseWriter, r *http.Request, name, message string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		writeJSON(w, 400, map[string]string{"error_code": "VALIDATION", "message": message})
		return uuid.Nil, false
	}
	return id, true
}

func queryInt(w http.ResponseWriter, r *http.Request, name string) (int, bool) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return 0, true
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error_code": "VALIDATION", "message": "invalid " + name})
		return 0, false
	}
	return v, true
}

func pageLimit(limit, def, max int) int {
	if limit <= 0 {
		return def
	}
	if limit > max {
		return max
	}
	return limit
}
