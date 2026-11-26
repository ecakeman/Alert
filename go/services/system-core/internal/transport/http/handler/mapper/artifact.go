package mapper

import (
	"encoding/json"
	"log"

	apptask "github.com/ecakeman/alert/services/system-core/internal/application/task"
	"github.com/ecakeman/alert/services/system-core/internal/domain"
	"github.com/ecakeman/alert/services/system-core/internal/transport/http/dto"
)

// ToEpisodeArtifact 映射路由 16 单条内容；仅 DIAGNOSIS_REPORT 尝试解析 content_json。
func ToEpisodeArtifact(view apptask.EpisodeArtifactView) dto.EpisodeArtifact {
	out := dto.EpisodeArtifact{
		ArtifactType: view.ArtifactType,
		Version:      view.ArtifactVersion,
		ContentType:  view.ContentType,
		ContentText:  string(view.Content),
		ContentHash:  view.ContentHash,
	}
	if view.ArtifactType == string(domain.ArtifactDiagnosisReport) {
		var parsed map[string]any
		if err := json.Unmarshal(view.Content, &parsed); err == nil {
			out.ContentJSON = parsed
		} else {
			log.Printf("WARN artifact content is not valid json episode_id=%s artifact_type=%s", view.EpisodeID, view.ArtifactType)
		}
	}
	return out
}
