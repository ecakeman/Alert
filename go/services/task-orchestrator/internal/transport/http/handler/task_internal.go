package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ecakeman/alert/services/task-orchestrator/internal/application/apperr"
	taskapp "github.com/ecakeman/alert/services/task-orchestrator/internal/application/task"
	porttask "github.com/ecakeman/alert/services/task-orchestrator/internal/application/port/task"
	"github.com/ecakeman/alert/services/task-orchestrator/internal/domain"
)

// Internal 承接 episode/BFF 依赖的内部 API（P5.6：产物与人工重试）。
type Internal struct {
	svc *taskapp.Service
}

func NewInternal(svc *taskapp.Service) *Internal { return &Internal{svc: svc} }

func (h *Internal) Register(router *gin.Engine) {
	g := router.Group("/internal")
	g.GET("/episodes/:episode_id/active-task", h.activeTask)
	g.GET("/episodes/:episode_id/artifact-readiness", h.readiness)
	g.GET("/episodes/:episode_id/artifacts", h.artifacts)
	g.POST("/episodes/:episode_id/retry-diagnosis", h.retryDiagnosis)
	g.POST("/episodes/:episode_id/retry-archive", h.retryArchive)
}

func (h *Internal) activeTask(c *gin.Context) {
	view, err := h.svc.ActiveTaskFor(c.Request.Context(), c.Param("episode_id"))
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"task_id": view.TaskID, "episode_id": view.EpisodeID, "service_id": view.ServiceID,
		"task_type": view.TaskType, "status": view.Status, "generation": view.Generation,
	})
}

func (h *Internal) readiness(c *gin.Context) {
	r, err := h.svc.ArtifactReadiness(c.Request.Context(), c.Param("episode_id"))
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"investigation": r.Investigation, "diagnosis_report": r.DiagnosisReport, "hashes_match": r.HashesMatch,
	})
}

type artifactRefDTO struct {
	ArtifactType    string `json:"artifact_type"`
	ArtifactVersion int    `json:"artifact_version"`
	StorageKey      string `json:"storage_key"`
	ContentType     string `json:"content_type"`
	ContentHash     string `json:"content_hash"`
	UpdatedAt       string `json:"updated_at"`
}

func (h *Internal) artifacts(c *gin.Context) {
	episodeID := c.Param("episode_id")
	artifactType := c.Query("type")
	if artifactType == "" {
		refs, err := h.svc.ListArtifactRefs(c.Request.Context(), episodeID)
		if err != nil {
			writeErr(c, err)
			return
		}
		out := make([]artifactRefDTO, 0, len(refs))
		for _, ref := range refs {
			out = append(out, toRefDTO(ref))
		}
		c.JSON(http.StatusOK, gin.H{"artifacts": out})
		return
	}
	if artifactType != string(domain.ArtifactInvestigation) && artifactType != string(domain.ArtifactDiagnosisReport) {
		c.JSON(http.StatusBadRequest, gin.H{"code": string(domain.CodeUnknownArtifact), "message": "unknown artifact type"})
		return
	}
	view, err := h.svc.ReadArtifact(c.Request.Context(), episodeID, artifactType)
	if err != nil {
		writeErr(c, err)
		return
	}
	body := gin.H{
		"artifact_type": view.ArtifactType, "version": view.ArtifactVersion,
		"content_type": view.ContentType, "content_hash": view.ContentHash,
	}
	if view.ContentType == "application/json" {
		var payload map[string]any
		if err := json.Unmarshal(view.Content, &payload); err == nil {
			body["content_json"] = payload
		} else {
			body["content_text"] = string(view.Content)
		}
	} else {
		body["content_text"] = string(view.Content)
	}
	c.JSON(http.StatusOK, body)
}

func (h *Internal) retryDiagnosis(c *gin.Context) {
	view, err := h.svc.RetryDiagnosis(c.Request.Context(), c.Param("episode_id"))
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusAccepted, toAccepted(view))
}

func (h *Internal) retryArchive(c *gin.Context) {
	view, err := h.svc.RetryArchive(c.Request.Context(), c.Param("episode_id"))
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusAccepted, toAccepted(view))
}

func toRefDTO(ref porttask.ArtifactRef) artifactRefDTO {
	return artifactRefDTO{
		ArtifactType: ref.ArtifactType, ArtifactVersion: ref.ArtifactVersion,
		StorageKey: ref.StorageKey, ContentType: ref.ContentType,
		ContentHash: ref.ContentHash, UpdatedAt: ref.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func toAccepted(view taskapp.TaskView) gin.H {
	return gin.H{
		"task_id": view.TaskID, "episode_id": view.EpisodeID, "task_type": view.TaskType,
		"status": view.Status, "generation": view.Generation, "attempt": view.Attempt,
		"created_at": view.CreatedAt.UTC().Format(time.RFC3339Nano), "reused": false,
	}
}

func writeErr(c *gin.Context, err error) {
	code := apperr.Code(err)
	c.JSON(apperr.HTTPStatus(code), map[string]string{"code": code, "message": err.Error()})
}