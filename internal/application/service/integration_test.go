package service_test

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

func poolAndUC(t *testing.T) *appservice.Service {
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
	return appservice.New(appservice.Deps{
		UOW:      postgres.NewUnitOfWork(pool),
		Services: postgres.NewServiceRepo(),
		Targets:  postgres.NewTargetRepo(),
		Sources:  postgres.NewSourceStateRepo(),
		Outbox:   postgres.NewOutboxRepo(),
		Clock:    clock{},
		IDs:      ids{},
	})
}

func TestCreateServiceAndDuplicate(t *testing.T) {
	uc := poolAndUC(t)
	ctx := context.Background()
	in := appservice.CreateServiceInput{
		ServiceKey: fmt.Sprintf("k-%d", time.Now().UnixNano()),
		Name:       "n", AuthorityProducerID: "p1",
	}
	view, err := uc.Create(ctx, in)
	if err != nil || view.Service.ServiceID == "" {
		t.Fatalf("create failed: %v", err)
	}
	_, err = uc.Create(ctx, in)
	if !isCode(err, apperr.CodeAlreadyExists) {
		t.Fatalf("expected ALREADY_EXISTS, got %v", err)
	}
}

func TestGetNotFound(t *testing.T) {
	uc := poolAndUC(t)
	_, err := uc.Get(context.Background(), "10000000-0000-0000-0000-999999999999")
	if !isCode(err, apperr.CodeNotFound) {
		t.Fatalf("expected NOT_FOUND, got %v", err)
	}
}