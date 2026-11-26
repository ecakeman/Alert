package httperr

import (
	"errors"
	"net/http"
	"testing"

	"github.com/ecakeman/alert/services/system-core/internal/application/apperr"
	"github.com/ecakeman/alert/services/system-core/internal/domain"
)

// 六档映射：apperr 与 domain 双来源各至少 1 例。
func TestStatusTable(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"apperr validation", apperr.Validation("bad input"), http.StatusBadRequest},
		{"domain invalid range", domain.ValidateBound(domain.InputBound{}), http.StatusBadRequest},
		{"apperr not found", apperr.NotFound("missing"), http.StatusNotFound},
		{"domain not found", &domain.Error{Code: domain.CodeNotFound, Message: "gone"}, http.StatusNotFound},
		{"apperr artifact not found", apperr.ArtifactNotFound("no artifact"), http.StatusNotFound},
		{"apperr already exists", apperr.AlreadyExists("dup"), http.StatusConflict},
		{"domain invalid transition", domain.CanTransitionIncident(domain.IncidentActive, domain.IncidentArchived), http.StatusConflict},
		{"apperr not implemented", apperr.NotImplemented("todo"), http.StatusNotImplemented},
		{"apperr dependency unavailable", apperr.DependencyUnavailable("db down"), http.StatusServiceUnavailable},
		{"unknown error", errors.New("boom"), http.StatusInternalServerError},
	}

	for _, c := range cases {
		if got := Status(c.err); got != c.want {
			t.Fatalf("%s: expected %d, got %d", c.name, c.want, got)
		}
	}
}

func TestClassifyStableCode(t *testing.T) {
	status, code, _ := Classify(apperr.NotFound("x"))
	if status != http.StatusNotFound || code != apperr.CodeNotFound {
		t.Fatalf("classify apperr: status=%d code=%s", status, code)
	}
	_, code, _ = Classify(errors.New("boom"))
	if code != "INTERNAL" {
		t.Fatalf("unknown should map INTERNAL, got %s", code)
	}
}

// T-S1.14：domain 错误归一后 code/status 不变。
func TestDomainErrorNormalizeKeepsCodeAndStatus(t *testing.T) {
	err := domain.CanTransitionIncident(domain.IncidentActive, domain.IncidentArchived)
	status, code, _ := Classify(apperr.Normalize(err))
	if status != http.StatusConflict || code != domain.CodeInvalidTransition {
		t.Fatalf("normalize should keep code/status: status=%d code=%s", status, code)
	}
}
