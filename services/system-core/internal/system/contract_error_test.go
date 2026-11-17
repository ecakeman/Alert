package system

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// TestCreateServiceDuplicateKeyIsConflict 覆盖契约偏差 #3：
// 重复的 service_key 必须返回 409 ALREADY_EXISTS，而不是 500 INTERNAL。
// 这里直接打真实测试库（migrations 建的 uniq_services_service_key_active），
// 验证 store.go 里 mapDBErr 的接入位置是对的，而不只是 mapDBErr 本身。
func TestCreateServiceDuplicateKeyIsConflict(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	in := CreateServiceInput{
		ServiceKey: "dup-" + uuid.NewString()[:8], Name: "orders", AuthorityProducerID: "order-api",
		Targets: []TargetInput{{
			TargetKey: "api", TargetName: "API", TargetType: "HTTP", Address: "http://orders/health",
			IntervalMS: 1000, TimeoutMS: 200, FailureThreshold: 2, RecoveryThreshold: 2,
		}},
	}
	if _, err := s.CreateService(ctx, in); err != nil {
		t.Fatalf("first CreateService: %v", err)
	}
	_, err := s.CreateService(ctx, in)
	if err == nil {
		t.Fatal("duplicate service_key was accepted")
	}
	if !IsCode(err, "ALREADY_EXISTS") {
		t.Fatalf("IsCode(ALREADY_EXISTS) = false, err = %v", err)
	}
	if e, ok := err.(*Error); !ok || e.HTTP != 409 {
		t.Fatalf("duplicate service_key error = %#v, want *Error with HTTP 409", err)
	}
}

// TestRetryUnknownEpisodeIsNotFound 覆盖契约偏差 #4：
// retry-diagnosis / retry-archive 对不存在的 episode 必须返回 404 EPISODE_NOT_FOUND，
// 而不是把 sql.ErrNoRows 透出成 500 INTERNAL。
func TestRetryUnknownEpisodeIsNotFound(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	unknown := uuid.New()

	if _, err := s.RetryDiagnosis(ctx, unknown); !IsCode(err, "EPISODE_NOT_FOUND") {
		t.Fatalf("RetryDiagnosis error = %v, want EPISODE_NOT_FOUND", err)
	} else if e, ok := err.(*Error); !ok || e.HTTP != 404 {
		t.Fatalf("RetryDiagnosis error = %#v, want *Error with HTTP 404", err)
	}

	if _, err := s.RetryArchive(ctx, unknown); !IsCode(err, "EPISODE_NOT_FOUND") {
		t.Fatalf("RetryArchive error = %v, want EPISODE_NOT_FOUND", err)
	} else if e, ok := err.(*Error); !ok || e.HTTP != 404 {
		t.Fatalf("RetryArchive error = %#v, want *Error with HTTP 404", err)
	}
}
