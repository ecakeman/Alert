package probe

import (
	"context"
	"net"
	"time"

	portService "github.com/ecakeman/alert/services/monitor-svc/internal/application/port/service"
)

// TCPProber 用真实 net.Dialer 探测 TCP 连通性。
type TCPProber struct{}

func NewTCPProber() *TCPProber { return &TCPProber{} }

func (p *TCPProber) Probe(ctx context.Context, target portService.MonitorTarget) Sample {
	timeout := time.Duration(target.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	dialer := &net.Dialer{Timeout: timeout}

	start := time.Now()
	conn, err := dialer.DialContext(ctx, "tcp", target.Address)
	lat := int(time.Since(start).Milliseconds())
	if err != nil {
		if ne, ok := err.(net.Error); ok && ne.Timeout() {
			return Sample{CheckStatus: "TIMEOUT", StatusClass: "timeout", ErrorClass: "timeout", LatencyMS: latencyMS(lat), StateFingerprint: fingerprint("timeout", "timeout")}
		}
		return Sample{CheckStatus: "FAILURE", StatusClass: "connection_error", ErrorClass: "connect", LatencyMS: latencyMS(lat), StateFingerprint: fingerprint("connection_error", "connect")}
	}
	_ = conn.Close()
	return Sample{CheckStatus: "SUCCESS", LatencyMS: latencyMS(lat), StateFingerprint: fingerprint("tcp_ok")}
}
