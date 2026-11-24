package bootstrap

import (
	"context"
	"time"

	appmonitor "github.com/ecakeman/alert/internal/application/monitor"
	appoutbox "github.com/ecakeman/alert/internal/application/outbox"
	portmonitor "github.com/ecakeman/alert/internal/application/port/monitor"
	"github.com/ecakeman/alert/internal/infrastructure/messaging"
	"github.com/ecakeman/alert/internal/infrastructure/probe"
)

// Worker 承载后台角色装配：probe 引擎 + 可选 Sender/Publisher/Consumer。
// 角色白名单：仅后台装配，不含 HTTP 服务/路由/迁移（出口检查 rg 断言）；
// OutboxPublisherEnabled=false 时空态仅探测引擎。
type Worker struct {
	Probe     *probe.Engine
	Sender    *messaging.Sender
	Publisher *appoutbox.Publisher
	Consumer  *messaging.Consumer
}

func NewWorker(sh *Shared, r *Repos, ucs *UseCases) (*Worker, error) {
	engine := probe.NewEngine(probe.EngineDeps{
		Source: ucs.Monitor,
		HTTP:   probe.NewHTTPProber(), TCP: probe.NewTCPProber(),
		Handler: func(ctx context.Context, target portmonitor.TargetSnapshot, smp probe.Sample) error {
			_, err := ucs.Monitor.ApplySample(ctx, target.ServiceID, target.TargetID, target.ConfigGeneration, appmonitor.Sample{
				CheckStatus: smp.CheckStatus, StatusClass: smp.StatusClass, ErrorClass: smp.ErrorClass,
				LatencyMS: smp.LatencyMS, StateFingerprint: smp.StateFingerprint,
			})
			return err
		},
		Workers: sh.Cfg.ProbeWorkers, Capacity: sh.Cfg.ProbeQueueCapacity, Clock: sh.Clock, ScanEvery: time.Second,
	})

	var sender *messaging.Sender
	var publisher *appoutbox.Publisher
	var consumer *messaging.Consumer
	if sh.Cfg.OutboxPublisherEnabled {
		brokers := splitBrokers(sh.Cfg.KafkaBrokers)
		sender = messaging.NewSender(brokers, sh.Cfg.KafkaTopic)
		publisher = appoutbox.New(appoutbox.Deps{
			UOW: sh.UOW, Outbox: r.Outbox, Sender: sender, Clock: sh.Clock,
			Owner: "system-core", Batch: sh.Cfg.OutboxBatch,
			ClaimLease: sh.Cfg.OutboxClaimLease, PollInterval: sh.Cfg.OutboxPollInterval,
		})
		consumer = messaging.NewConsumer(brokers, sh.Cfg.KafkaTopic, "system.audit.v1",
			func(ctx context.Context, body []byte) error {
				ev, err := messaging.Decode(body)
				if err != nil {
					return err
				}
				return ucs.AuditConsumer.Handle(ctx, ev)
			})
	}

	return &Worker{Probe: engine, Sender: sender, Publisher: publisher, Consumer: consumer}, nil
}
