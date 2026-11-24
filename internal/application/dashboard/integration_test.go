package dashboard_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	appdashboard "github.com/ecakeman/alert/internal/application/dashboard"
	"github.com/ecakeman/alert/internal/infrastructure/persistence/postgres"
)

type clock struct{}

func (clock) Now() time.Time { return time.Now().UTC() }

func TestSummary(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("未执行（环境：无 TEST_DATABASE_URL）")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	uc := appdashboard.New(appdashboard.Deps{
		UOW: postgres.NewUnitOfWork(pool), Repo: postgres.NewDashboardRepo(), Clock: clock{},
	})
	sum, err := uc.Summary(context.Background())
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if sum.Services == nil || sum.Targets == nil || sum.Episodes == nil {
		t.Fatalf("summary maps should be non-nil: %+v", sum)
	}
	if sum.Recent == nil || sum.Trend == nil {
		t.Fatalf("recent/trend slices should be non-nil (empty as []): %+v", sum)
	}
}
