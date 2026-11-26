package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestArtifactRouteRejectsUnknownType(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	artifact := &Artifact{}
	router.GET("/episodes/:episode_id/artifacts", artifact.Get)

	request := httptest.NewRequest(http.MethodGet, "/episodes/e1/artifacts?type=UNKNOWN", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("unknown type should be 400, got %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "UNKNOWN_ARTIFACT_TYPE") {
		t.Fatalf("body must carry stable code, got %s", recorder.Body.String())
	}
}
