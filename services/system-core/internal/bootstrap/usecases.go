package bootstrap

import (
	"context"
	"strings"
	"time"

	appdashboard "github.com/ecakeman/alert/services/system-core/internal/application/dashboard"
	appepisode "github.com/ecakeman/alert/services/system-core/internal/application/episode"
	appmonitor "github.com/ecakeman/alert/services/system-core/internal/application/monitor"
	appoutbox "github.com/ecakeman/alert/services/system-core/internal/application/outbox"
	portdispatch "github.com/ecakeman/alert/services/system-core/internal/application/port/dispatch"
	appservice "github.com/ecakeman/alert/services/system-core/internal/application/service"
	apptask "github.com/ecakeman/alert/services/system-core/internal/application/task"
	"github.com/ecakeman/alert/services/system-core/internal/infrastructure/artifacts"
)

// UseCases 是六个应用用例的集合。
// AuditConsumer 无条件构造：纯依赖集合、无 IO 副作用；messaging 组件仍按开关分支构造。
type UseCases struct {
	Service       *appservice.Service
	Dashboard     *appdashboard.Service
	Episode       *appepisode.Service
	Monitor       *appmonitor.Service
	Task          *apptask.Service
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
		Outbox:   r.Outbox,
		Driver:   episodeDriverAdapter{uc: epUC},
		Clock:    sh.Clock,
		IDs:      sh.IDs,
	})

	auditUC := appoutbox.NewConsumer(appoutbox.ConsumerDeps{
		UOW: sh.UOW, Idem: r.Idem, Clock: sh.Clock,
	})

	taskUC := newTaskService(sh, r, nil)

	return &UseCases{
		Service: svcUC, Dashboard: dashUC, Episode: epUC,
		Monitor: monitorUC, Task: taskUC, AuditConsumer: auditUC,
	}
}

func newTaskService(sh *Shared, r *Repos, submit portdispatch.TaskSubmitter) *apptask.Service {
	return apptask.New(apptask.Deps{
		UOW: sh.UOW, Tasks: r.Tasks, Ownership: r.Ownership, Artifacts: r.Artifacts,
		Episodes: r.Episodes, InputVersions: r.InputVersions,
		Outbox: r.Outbox, Idem: r.Idem,
		Submit: submit, Reader: artifacts.NewReader(),
		Clock: sh.Clock, IDs: sh.IDs,
		Lease: taskLease(sh.Cfg.TaskLease, sh.Cfg.OwnershipTTL), MaxAttempts: sh.Cfg.TaskMaxAttempts,
		RetryDelay: sh.Cfg.TaskRetryDelay, Deadline: sh.Cfg.TaskDeadline,
	})
}

func taskLease(taskLease, ownershipTTL time.Duration) time.Duration {
	if ownershipTTL > 0 && ownershipTTL < taskLease {
		return ownershipTTL
	}
	return taskLease
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
