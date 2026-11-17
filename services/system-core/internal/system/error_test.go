package system

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// TestMapDBErrUniqueViolationTo409 覆盖「重复 service_key 返回 500 INTERNAL」这条
// 契约偏差的修复：pgx 的 23505 唯一约束冲突必须翻译成 409 业务错误。
func TestMapDBErrUniqueViolationTo409(t *testing.T) {
	raw := &pgconn.PgError{Code: "23505", ConstraintName: "uniq_services_service_key_active"}
	err := mapDBErr(raw, "ALREADY_EXISTS", "service_key already exists")
	if err == nil {
		t.Fatal("mapDBErr returned nil for a unique violation")
	}
	if !IsCode(err, "ALREADY_EXISTS") {
		t.Fatalf("IsCode(ALREADY_EXISTS) = false, err = %v", err)
	}
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("mapped error is not *Error: %T", err)
	}
	if e.HTTP != 409 {
		t.Fatalf("HTTP = %d, want 409", e.HTTP)
	}
}

// TestMapDBErrPassesThroughOthers：非唯一约束冲突保持原样，由上层决定是否 500。
func TestMapDBErrPassesThroughOthers(t *testing.T) {
	raw := &pgconn.PgError{Code: "42P01"}
	err := mapDBErr(raw, "ALREADY_EXISTS", "service_key already exists")
	if err == nil {
		t.Fatal("mapDBErr returned nil")
	}
	if IsCode(err, "ALREADY_EXISTS") {
		t.Fatalf("non-23505 error was translated: %v", err)
	}
	if !errors.Is(err, raw) {
		t.Fatal("non-23505 error was not passed through unchanged")
	}
	// 非 pg 错误同样透传。
	plain := errors.New("boom")
	if got := mapDBErr(plain, "ALREADY_EXISTS", "x"); !errors.Is(got, plain) {
		t.Fatalf("plain error was replaced: %v", got)
	}
	if got := mapDBErr(nil, "ALREADY_EXISTS", "x"); got != nil {
		t.Fatalf("mapDBErr(nil) = %v, want nil", got)
	}
}
