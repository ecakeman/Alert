package probe

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/ecakeman/alert/internal/application/port"
)

// HTTPProber 用真实 http.Client 探测目标地址。
type HTTPProber struct {
	client *http.Client
}

func NewHTTPProber() *HTTPProber { return &HTTPProber{client: &http.Client{}} }

func (p *HTTPProber) Probe(ctx context.Context, target port.MonitorTarget) Sample {
	timeout := time.Duration(target.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, target.Address, nil)
	if err != nil {
		return Sample{CheckStatus: "ERROR", StatusClass: "invalid_request", ErrorClass: "request_build", StateFingerprint: fingerprint("invalid_request", "request_build")}
	}

	start := time.Now()
	resp, err := p.client.Do(req)
	lat := int(time.Since(start).Milliseconds())
	if err != nil {
		if reqCtx.Err() == context.DeadlineExceeded {
			return Sample{CheckStatus: "TIMEOUT", StatusClass: "timeout", ErrorClass: "timeout", LatencyMS: latencyMS(lat), StateFingerprint: fingerprint("timeout", "timeout")}
		}
		return Sample{CheckStatus: "FAILURE", StatusClass: "connection_error", ErrorClass: "connection", LatencyMS: latencyMS(lat), StateFingerprint: fingerprint("connection_error", "connection")}
	}
	defer resp.Body.Close()

	class := strconv.Itoa(resp.StatusCode/100) + "xx"
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return Sample{CheckStatus: "SUCCESS", StatusClass: class, LatencyMS: latencyMS(lat), StateFingerprint: fingerprint(class)}
	}
	return Sample{CheckStatus: "FAILURE", StatusClass: class, ErrorClass: "http_status", LatencyMS: latencyMS(lat), StateFingerprint: fingerprint(class, "http_status")}
}
