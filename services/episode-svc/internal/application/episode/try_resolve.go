package episode

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/ecakeman/alert/services/contracts"

	"github.com/ecakeman/alert/services/episode-svc/internal/application/apperr"
	portepisode "github.com/ecakeman/alert/services/episode-svc/internal/application/port/episode"
	portshared "github.com/ecakeman/alert/services/episode-svc/internal/application/port/shared"
	"github.com/ecakeman/alert/services/episode-svc/internal/domain"
)

// TryResolve：满足全部解除条件才置 RESOLVED（INV-03）。
// 判定输入全部在事务前由 registry/task 取得（禁止事务内远程调用），写侧用 CAS
// （仍 ACTIVE 且 updated_at 未被并发修改）保证“并发只有 1 个成功”。
func (s *Service) TryResolve(ctx context.Context, episodeID string) (bool, []string, error) {
	var ep portepisode.Episode
	if err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		var err error
		ep, err = s.deps.Episodes.Get(ctx, tx, episodeID)
		return err
	}); err != nil {
		return false, nil, err
	}

	sources, err := s.deps.Registry.ListSourceStates(ctx, ep.ServiceID)
	if err != nil {
		slog.Default().WarnContext(ctx, "resolve precheck failed",
			"episode_id", episodeID, "domain", "registry", "reason", err.Error())
		return false, nil, apperr.DependencyUnavailable("resolve precheck unavailable")
	}
	activeTask, err := s.deps.Tasks.GetActive(ctx, episodeID)
	if err != nil {
		slog.Default().WarnContext(ctx, "resolve precheck failed",
			"episode_id", episodeID, "domain", "task", "reason", err.Error())
		return false, nil, apperr.DependencyUnavailable("resolve precheck unavailable")
	}

	monitorNormal, appNormal := false, false
	for _, st := range sources {
		switch st.SourceType {
		case "MONITOR":
			monitorNormal = st.Status == "NORMAL"
		case "APP":
			appNormal = st.Status == "NORMAL"
		}
	}
	finalProcessed := ep.FinalInputVersion > 0 && ep.ProcessedInputVersion == ep.FinalInputVersion
	ok, miss := domain.CanResolve(domain.ResolveInput{
		MonitorNormal:       monitorNormal,
		AppNormal:           appNormal,
		FinalInputProcessed: finalProcessed,
		ActiveTaskExists:    activeTask != nil,
	})
	missing := make([]string, 0, len(miss))
	for _, m := range miss {
		missing = append(missing, string(m))
	}
	if !ok {
		return false, missing, nil
	}

	var resolved bool
	now := s.deps.Clock.Now().UTC()
	err = s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx portshared.Tx) error {
		applied, err := s.deps.Episodes.ResolveIfUnchanged(ctx, tx, episodeID, ep.UpdatedAt, now)
		if err != nil {
			return err
		}
		if !applied {
			slog.Default().WarnContext(ctx, "resolve skipped by CAS conflict", "episode_id", episodeID)
			return nil
		}
		resolved = true
		return s.appendEvent(ctx, tx, contracts.EventEpisodeStatusChanged, "episode", episodeID,
			map[string]string{"episode_id": episodeID, "status": "RESOLVED"})
	})
	if err != nil {
		return false, missing, err
	}
	return resolved, missing, nil
}

var _ = fmt.Sprintf