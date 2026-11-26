package handler

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ecakeman/alert/services/system-core/internal/application/apperr"
	"github.com/ecakeman/alert/services/system-core/internal/transport/http/httperr"
	"github.com/ecakeman/alert/services/system-core/internal/transport/http/response"
)

func writeErr(c *gin.Context, err error) {
	status, code, msg := httperr.Classify(apperr.Normalize(err))
	response.Error(c, status, code, msg)
}

func parseLimit(raw string, def int) (int, bool) {
	if raw == "" {
		return def, true
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 || n > 200 {
		return 0, false
	}
	return n, true
}

func parseOffset(raw string) (int, bool) {
	if raw == "" {
		return 0, true
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

func fmtTime(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.UTC().Format(time.RFC3339)
	return &s
}
