package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	apptask "github.com/ecakeman/alert/services/system-core/internal/application/task"
	"github.com/ecakeman/alert/services/system-core/internal/domain"
	"github.com/ecakeman/alert/services/system-core/internal/transport/http/dto"
	"github.com/ecakeman/alert/services/system-core/internal/transport/http/handler/mapper"
	"github.com/ecakeman/alert/services/system-core/internal/transport/http/response"
)

// Artifact 承载路由 16：按 type 读取单条产物或列出元数据。
type Artifact struct{ uc *apptask.Service }

func NewArtifact(uc *apptask.Service) *Artifact { return &Artifact{uc: uc} }

func (h *Artifact) Get(c *gin.Context) {
	episodeID := c.Param("episode_id")
	artifactType := c.Query("type")
	if artifactType == "" {
		refs, err := h.uc.ListArtifactRefs(c.Request.Context(), episodeID)
		if err != nil {
			writeErr(c, err)
			return
		}
		response.JSON(c, http.StatusOK, dto.ArtifactList{Artifacts: mapper.ToArtifactRefs(refs)})
		return
	}
	if !validArtifactType(artifactType) {
		response.Error(c, http.StatusBadRequest, string(domain.CodeUnknownArtifact), "unknown artifact type")
		return
	}
	view, err := h.uc.ReadArtifact(c.Request.Context(), episodeID, artifactType)
	if err != nil {
		writeErr(c, err)
		return
	}
	response.JSON(c, http.StatusOK, mapper.ToEpisodeArtifact(view))
}

func validArtifactType(value string) bool {
	return value == string(domain.ArtifactInvestigation) || value == string(domain.ArtifactDiagnosisReport)
}
