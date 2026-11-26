package dispatch

import (
	"context"

	porttask "github.com/ecakeman/alert/services/system-core/internal/application/port/task"
)

// TaskSubmitter：事务外提交；accepted 仅表示对方按协议受理。
type TaskSubmitter interface {
	SubmitTask(ctx context.Context, t porttask.AgentTask) (accepted bool, err error)
}

// ArtifactContent：正文 + 类型；无正文必须返回错误，不得返回空串。
type ArtifactContent struct {
	Text        []byte
	ContentType string
}

// ArtifactContentReader：正文读取（B8 前为受限实现）。
type ArtifactContentReader interface {
	ReadArtifact(ctx context.Context, ref porttask.ArtifactRef) (ArtifactContent, error)
}
