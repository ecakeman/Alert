package monitorclient

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ecakeman/alert/services/platform/apierr"
	"github.com/ecakeman/alert/services/platform/httpx"

	"github.com/ecakeman/alert/services/episode-svc/internal/application/apperr"
	portmonitor "github.com/ecakeman/alert/services/episode-svc/internal/application/port/monitor"
)

// Client 是 monitor-svc 的域间 HTTP 客户端（只读点位，事务外调用）。
type Client struct {
	http *httpx.Client
}

func New(baseURL string, timeout time.Duration) *Client {
	return &Client{http: httpx.NewClient(baseURL, timeout)}
}

type latestSeqDTO struct {
	LatestSeq int64 `json:"latest_seq"`
}

type pointDTO struct {
	PointSeq         int64  `json:"point_seq"`
	TargetID         string `json:"target_id"`
	TargetKey        string `json:"target_key"`
	TargetType       string `json:"target_type"`
	ObservedAt       string `json:"observed_at"`
	CheckStatus      string `json:"check_status"`
	StatusClass      string `json:"status_class"`
	ErrorClass       string `json:"error_class"`
	StateFingerprint string `json:"state_fingerprint"`
}

func (c *Client) LatestSeqByService(ctx context.Context, serviceID string) (int64, error) {
	var dto latestSeqDTO
	if err := c.http.GetJSON(ctx, "/internal/services/"+serviceID+"/points/latest-seq", &dto); err != nil {
		return 0, mapErr(err)
	}
	return dto.LatestSeq, nil
}

func (c *Client) ListByBound(ctx context.Context, serviceID string, fromSeq, toSeq int64, limit int) ([]portmonitor.MonitorPoint, error) {
	path := fmt.Sprintf("/internal/services/%s/points?from_seq=%d&to_seq=%d&limit=%d", serviceID, fromSeq, toSeq, limit)
	var dtos []pointDTO
	if err := c.http.GetJSON(ctx, path, &dtos); err != nil {
		return nil, mapErr(err)
	}
	out := make([]portmonitor.MonitorPoint, 0, len(dtos))
	for _, d := range dtos {
		p := portmonitor.MonitorPoint{
			PointSeq: d.PointSeq, TargetID: d.TargetID, TargetKey: d.TargetKey, TargetType: d.TargetType,
			CheckStatus: d.CheckStatus, StatusClass: d.StatusClass, ErrorClass: d.ErrorClass,
			StateFingerprint: d.StateFingerprint,
		}
		if parsed, err := time.Parse(time.RFC3339Nano, d.ObservedAt); err == nil {
			p.ObservedAt = parsed
		}
		out = append(out, p)
	}
	return out, nil
}

func mapErr(err error) error {
	if err == nil {
		return nil
	}
	var ae *apierr.Error
	if errors.As(err, &ae) {
		switch ae.Code {
		case apierr.CodeNotFound:
			return apperr.NotFound(ae.Message)
		case apierr.CodeValidation:
			return apperr.Validation(ae.Message)
		case apierr.CodeDependencyUnavailable:
			return apperr.DependencyUnavailable(ae.Message)
		}
		return apperr.New(ae.Code, ae.Message)
	}
	return apperr.DependencyUnavailable("monitor unavailable")
}