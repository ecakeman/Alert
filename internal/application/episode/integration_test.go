package episode_test

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ecakeman/alert/internal/application/apperr"
	"github.com/ecakeman/alert/internal/application/port"
	appepisode "github.com/ecakeman/alert/internal/application/episode"
	appservice "github.com/ecakeman/alert/internal/application/service"
	"github.com/ecakeman/alert/internal/infrastructure/persistence/postgres"
)

type clock struct{}

func (clock) Now() time.Time { return time.Now().UTC() }

type ids struct{}

func (ids) NewID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func isCode(err error, code string) bool {
	var ae *apperr.Error
	return errors.As(err, &ae) && ae.Code == code
}

func setup(t *testing.T) (*appepisode.Service, *appservice.Service) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("未执行（环境：无 TEST_DATABASE_URL）")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	uow := postgres.NewUnitOfWork(pool)
	clk := clock{}
	gen := ids{}
	svcUC := appservice.New(appservice.Deps{
		UOW: uow, Services: postgres.NewServiceRepo(), Targets: postgres.NewTargetRepo(),
		Sources: postgres.NewSourceStateRepo(), Outbox: postgres.NewOutboxRepo(), Clock: clk, IDs: gen,
	})
	epUC := appepisode.New(appepisode.Deps{
		UOW: uow, Episodes: postgres.NewEpisodeRepo(), InputVersions: postgres.NewInputVersionRepo(),
		AppEvents: postgres.NewAppEventRepo(), MonitorPoints: postgres.NewMonitorPointRepo(),
		Sources: postgres.NewSourceStateRepo(), Tasks: postgres.NewTaskRepo(),
		Outbox: postgres.NewOutboxRepo(), Clock: clk, IDs: gen,
	})
	return epUC, svcUC
}

func createService(t *testing.T, svcUC *appservice.Service) string {
	t.Helper()
	view, err := svcUC.Create(context.Background(), appservice.CreateServiceInput{
		ServiceKey: fmt.Sprintf("ep-%d", time.Now().UnixNano()),
		Name:       "n", AuthorityProducerID: "p1",
	})
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	return view.Service.ServiceID
}

func TestOpenOrReuse(t *testing.T) {
	epUC, svcUC := setup(t)
	ctx := context.Background()
	svcID := createService(t, svcUC)

	ep1, created1, err := epUC.OpenOrReuse(ctx, svcID)
	if err != nil || !created1 {
		t.Fatalf("first open should create: created=%v err=%v", created1, err)
	}
	ep2, created2, err := epUC.OpenOrReuse(ctx, svcID)
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	if created2 || ep2.EpisodeID != ep1.EpisodeID {
		t.Fatalf("second should reuse: created=%v id1=%s id2=%s", created2, ep1.EpisodeID, ep2.EpisodeID)
	}
}

func TestAppendAppEventIdempotentAndConflict(t *testing.T) {
	epUC, svcUC := setup(t)
	ctx := context.Background()
	svcID := createService(t, svcUC)

	in := appepisode.AppEventInput{
		ProducerID: "p1", EventID: "e1", EventKind: "HEALTH_STATUS",
		HealthStatus: "ABNORMAL", Payload: []byte(`{"a":1}`), EventTime: time.Now().UTC(),
	}
	first, err := epUC.AppendAppEvent(ctx, svcID, in)
	if err != nil || !first.Appended {
		t.Fatalf("first append: %+v %v", first, err)
	}
	second, err := epUC.AppendAppEvent(ctx, svcID, in)
	if err != nil || second.Appended {
		t.Fatalf("duplicate should be idempotent: %+v %v", second, err)
	}

	conflict := in
	conflict.HealthStatus = "NORMAL"
	_, err = epUC.AppendAppEvent(ctx, svcID, conflict)
	if !isCode(err, "EVENT_ID_CONFLICT") {
		t.Fatalf("expected EVENT_ID_CONFLICT, got %v", err)
	}
}

func TestAppendInputVersionIdempotent(t *testing.T) {
	epUC, svcUC := setup(t)
	ctx := context.Background()
	svcID := createService(t, svcUC)

	ep, _, err := epUC.OpenOrReuse(ctx, svcID)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	first, err := epUC.AppendInputVersion(ctx, ep.EpisodeID)
	if err != nil || !first {
		t.Fatalf("first input version should append: v=%v err=%v", first, err)
	}
	second, err := epUC.AppendInputVersion(ctx, ep.EpisodeID)
	if err != nil || second {
		t.Fatalf("same fingerprint should not append: v=%v err=%v", second, err)
	}
}

func TestTryResolveBlocked(t *testing.T) {
	epUC, svcUC := setup(t)
	ctx := context.Background()
	svcID := createService(t, svcUC)
	ep, _, err := epUC.OpenOrReuse(ctx, svcID)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	resolved, missing, err := epUC.TryResolve(ctx, ep.EpisodeID)
	if err != nil {
		t.Fatalf("try resolve: %v", err)
	}
	if resolved || len(missing) == 0 {
		t.Fatalf("initial should not resolve and list missing: resolved=%v missing=%v", resolved, missing)
	}
	_ = port.SourceState{}
}