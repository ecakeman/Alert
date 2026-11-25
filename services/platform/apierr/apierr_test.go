package apierr

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPStatusMapping(t *testing.T) {
	cases := []struct {
		code string
		want int
	}{
		{CodeValidation, http.StatusUnprocessableEntity},
		{CodeNotFound, http.StatusNotFound},
		{CodeAlreadyExists, http.StatusConflict},
		{CodeStateConflict, http.StatusConflict},
		{CodeDependencyUnavailable, http.StatusServiceUnavailable},
		{CodeNotImplemented, http.StatusNotImplemented},
		{CodeInternal, http.StatusInternalServerError},
		{"UNKNOWN", http.StatusInternalServerError},
	}
	for _, tc := range cases {
		if got := HTTPStatus(tc.code); got != tc.want {
			t.Errorf("HTTPStatus(%s) = %d，期望 %d", tc.code, got, tc.want)
		}
	}
}

func TestClassifyDoesNotLeakUnknownError(t *testing.T) {
	status, ae := Classify(errors.New("db password s3cret"))
	if status != http.StatusInternalServerError {
		t.Fatalf("status = %d", status)
	}
	if ae.Code != CodeInternal || ae.Message != "internal error" {
		t.Fatalf("Classify 结果 = %+v", ae)
	}
}

func TestWriteValidation(t *testing.T) {
	rec := httptest.NewRecorder()
	Write(rec, Validation("name required"))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d，期望 422", rec.Code)
	}
	var body Error
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("响应不是 JSON: %v", err)
	}
	if body.Code != CodeValidation || body.Message != "name required" {
		t.Fatalf("响应体 = %+v", body)
	}
}

func TestWriteNilIsNoContent(t *testing.T) {
	rec := httptest.NewRecorder()
	Write(rec, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d，期望 204", rec.Code)
	}
}

func TestWriteWrappedErrorKeepsCode(t *testing.T) {
	rec := httptest.NewRecorder()
	Write(rec, errors.Join(errors.New("outer"), NotFound("missing")))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d，期望 404", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"code":"NOT_FOUND"`) {
		t.Fatalf("响应体 = %s", rec.Body.String())
	}
}
