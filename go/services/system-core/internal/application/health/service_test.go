package health

import (
	"context"
	"errors"
	"testing"
)

type stubChecker struct {
	name string
	err  error
}

func (s stubChecker) Name() string                { return s.name }
func (s stubChecker) Check(context.Context) error { return s.err }

func TestReadyNoCheckers(t *testing.T) {
	s := NewService()
	if st := s.Ready(context.Background()); st.Status != "ready" {
		t.Fatalf("expected ready, got %+v", st)
	}
}

func TestReadyFailingChecker(t *testing.T) {
	s := NewService(stubChecker{name: "db", err: errors.New("down")})
	st := s.Ready(context.Background())
	if st.Status != "not_ready" {
		t.Fatalf("expected not_ready, got %+v", st)
	}
	if len(st.Checks) != 1 || st.Checks[0].Name != "db" || st.Checks[0].OK {
		t.Fatalf("expected failing db check, got %+v", st.Checks)
	}
}