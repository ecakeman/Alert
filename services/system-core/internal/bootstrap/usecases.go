package bootstrap

import (
	"context"
	"strings"

	appdashboard "github.com/ecakeman/alert/services/system-core/internal/application/dashboard"
	appepisode "github.com/ecakeman/alert/services/system-core/internal/application/episode"
	appmonitor "github.com/ecakeman/alert/services/system-core/internal/application/monitor"
	appoutbox "github.com/ecakeman/alert/services/system-core/internal/application/outbox"
	appservice "github.com/ecakeman/alert/services/system-core/internal/application/service"
)

// UseCases 是五个应用用例的集合。
// AuditConsumer 无条件构造：纯依赖集合、无 IO 副作用；messaging 组件仍按开关分支构造。
type UseCases struct {
	Service       *appservice.Service
	Dashboard     *appdashboard.Service
	Episode       *appepisode.Service
	Monitor       *appmonitor.Service
	AuditConsumer *appoutbox.Consumer
}

func NewUseCases(sh *Shared, r *Repos) *UseCases {
	svcUC := appservice.New(appservice.Deps{
		UOW: sh.UOW, Services: r.Services, Targets: r.Targets,
		Sources: r.Sources, Episodes: r.Episodes,
		Outbox: r.Outbox, Clock: sh.Clock, IDs: sh.IDs,
	})
	dashUC := appdashboard.New(appdashboard.Deps{
		UOW: sh.UOW, Repo: r.Dashboard, Clock: sh.Clock,
	})
	epUC := appepisode.New(appepisode.Deps{
		UOW: sh.UOW, Episodes: r.Episodes, InputVersions: r.InputVersions,
		AppEvents: r.AppEvents, MonitorPoints: r.MonitorPoints,
		Sources: r.Sources, Services: r.Services,
		Tasks: r.Tasks, Artifacts: r.Artifacts,
		Outbox: r.Outbox, Clock: sh.Clock, IDs: sh.IDs,
	})

	monitorUC := appmonitor.New(appmonitor.Deps{
		UOW: sh.UOW, Targets: r.Targets, MonitorPoints: r.MonitorPoints,
		Sources: r.Sources, Episodes: r.Episodes,
		Services: r.Services,
		Driver:   episodeDriverAdapter{uc: epUC},
		Clock:    sh.Clock,
	})

	auditUC := appoutbox.NewConsumer(appoutbox.ConsumerDeps{
		UOW: sh.UOW, Idem: r.Idem, Clock: sh.Clock,
	})

	return &UseCases{
		Service: svcUC, Dashboard: dashUC, Episode: epUC,
		Monitor: monitorUC, AuditConsumer: auditUC,
	}
}

// episodeDriverAdapter 把 episode 用例转调为 monitor.EpisodeDriver。
type episodeDriverAdapter struct{ uc *appepisode.Service }

func (a episodeDriverAdapter) OpenOrReuse(ctx context.Context, serviceID string) (bool, error) {
	_, created, err := a.uc.OpenOrReuse(ctx, serviceID)
	return created, err
}

func (a episodeDriverAdapter) TryResolve(ctx context.Context, episodeID string) (bool, []string, error) {
	return a.uc.TryResolve(ctx, episodeID)
}

func splitBrokers(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
