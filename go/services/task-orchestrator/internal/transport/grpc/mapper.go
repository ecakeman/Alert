package grpc

import (
	"encoding/json"

	systemv1 "github.com/ecakeman/alert/services/task-orchestrator/internal/transport/grpc/gen/alertflow/system/v1"

	apptask "github.com/ecakeman/alert/services/task-orchestrator/internal/application/task"
)

func toTaskIdentity(in *systemv1.TaskIdentity) apptask.TaskIdentity {
	if in == nil {
		return apptask.TaskIdentity{}
	}
	return apptask.TaskIdentity{
		TaskID:     in.TaskId,
		EpisodeID:  in.EpisodeId,
		TaskType:   in.TaskType,
		Generation: in.Generation,
		LeaseToken: in.LeaseToken,
	}
}

func fromTaskContext(in apptask.TaskContext) *systemv1.GetTaskContextResponse {
	return &systemv1.GetTaskContextResponse{
		InputVersionFrom:  in.InputVersionFrom,
		InputVersionTo:    in.InputVersionTo,
		MonitorWatermark:  in.MonitorWatermark,
		AppEventWatermark: in.AppEventWatermark,
		DeadlineAt:        in.DeadlineAt.Unix(),
		TaskType:          in.TaskType,
	}
}

func toRegisterArtifactInput(req *systemv1.RegisterTaskArtifactRequest) apptask.RegisterArtifactInput {
	var sourceRefs []string
	if len(req.SourceRefs) > 0 {
		_ = json.Unmarshal(req.SourceRefs, &sourceRefs)
	}
	return apptask.RegisterArtifactInput{
		Identity:        toTaskIdentity(req.Identity),
		ArtifactType:    req.ArtifactType,
		ArtifactVersion: int(req.ArtifactVersion),
		ContentHash:     req.ContentHash,
		ContentType:     req.ContentType,
		StorageKey:      req.StorageKey,
		SourceRefs:      sourceRefs,
	}
}
