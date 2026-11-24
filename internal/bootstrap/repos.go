package bootstrap

import (
	portdashboard "github.com/ecakeman/alert/internal/application/port/dashboard"
	portepisode "github.com/ecakeman/alert/internal/application/port/episode"
	portmonitor "github.com/ecakeman/alert/internal/application/port/monitor"
	portoutbox "github.com/ecakeman/alert/internal/application/port/outbox"
	portService "github.com/ecakeman/alert/internal/application/port/service"
	porttask "github.com/ecakeman/alert/internal/application/port/task"
	"github.com/ecakeman/alert/internal/infrastructure/persistence/postgres"
)

// Repos 集中持有全部仓储实例；纯实例化：不开事务、不构造用例。
// postgres 仓储为无状态结构体，单一实例复用与多次构造行为等价（spec 明令的装配收敛）。
type Repos struct {
	Services      portService.ServiceRepo
	Targets       portService.TargetRepo
	Sources       portService.SourceStateRepo
	Episodes      portepisode.EpisodeRepo
	InputVersions portepisode.InputVersionRepo
	AppEvents     portepisode.AppEventRepo
	MonitorPoints portmonitor.MonitorPointRepo
	Dashboard     portdashboard.DashboardRepo
	Outbox        portoutbox.OutboxRepo
	Idem          portoutbox.ConsumerIdempotencyRepo
	Tasks         porttask.TaskRepo
	Ownership     porttask.OwnershipRepo
	Artifacts     porttask.ArtifactRepo
}

func NewRepos(sh *Shared) *Repos {
	return &Repos{
		Services:      postgres.NewServiceRepo(),
		Targets:       postgres.NewTargetRepo(),
		Sources:       postgres.NewSourceStateRepo(),
		Episodes:      postgres.NewEpisodeRepo(),
		InputVersions: postgres.NewInputVersionRepo(),
		AppEvents:     postgres.NewAppEventRepo(),
		MonitorPoints: postgres.NewMonitorPointRepo(),
		Dashboard:     postgres.NewDashboardRepo(),
		Outbox:        postgres.NewOutboxRepo(),
		Idem:          postgres.NewIdempotencyRepo(),
		Tasks:         postgres.NewTaskRepo(),
		Ownership:     postgres.NewOwnershipRepo(),
		Artifacts:     postgres.NewArtifactRepo(),
	}
}
