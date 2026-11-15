package httpprobe

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/ecakeman/alertflow/services/system-core/internal/system"
)

var client = &http.Client{Transport: &http.Transport{
	TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
}}

func Probe(ctx context.Context, target system.DueTarget) system.MonitorSample {
	sample := system.MonitorSample{
		ServiceID: target.ServiceID, TargetID: target.TargetID, ObservedAt: time.Now().UTC(),
	}
	timeout := target.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, target.Address, nil)
	if err != nil {
		sample.StatusClass = "error"
		sample.ErrorClass = "connect"
		sample.Raw = rawErr(err)
		return sample
	}
	resp, err := client.Do(req)
	if err != nil {
		sample.StatusClass = "error"
		sample.ErrorClass = classify(err)
		sample.Raw = rawErr(err)
		return sample
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	sample.StatusClass = statusClass(resp.StatusCode)
	sample.Success = resp.StatusCode >= 200 && resp.StatusCode < 400
	if !sample.Success {
		sample.ErrorClass = "http_status"
	}
	raw, _ := json.Marshal(map[string]any{"status_code": resp.StatusCode, "body": string(body)})
	sample.Raw = raw
	return sample
}

func classify(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "timeout"
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "tls") || strings.Contains(msg, "x509") || strings.Contains(msg, "certificate") {
		return "tls"
	}
	return "connect"
}

func statusClass(code int) string {
	switch {
	case code >= 500:
		return "5xx"
	case code >= 400:
		return "4xx"
	case code >= 200:
		return "2xx"
	default:
		return "error"
	}
}

func rawErr(err error) []byte {
	raw, _ := json.Marshal(map[string]string{"error": err.Error()})
	return raw
}
