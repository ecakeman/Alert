package tcpprobe

import (
	"context"
	"encoding/json"
	"net"
	"time"

	"github.com/ecakeman/alertflow/services/system-core/internal/system"
)

func Probe(ctx context.Context, target system.DueTarget) system.MonitorSample {
	sample := system.MonitorSample{
		ServiceID: target.ServiceID, TargetID: target.TargetID, ObservedAt: time.Now().UTC(),
	}
	timeout := target.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", target.Address)
	if err != nil {
		sample.ErrorClass = "connect"
		raw, _ := json.Marshal(map[string]string{"error": err.Error()})
		sample.Raw = raw
		return sample
	}
	_ = conn.Close()
	sample.Success = true
	sample.Raw = []byte(`{"connected":true}`)
	return sample
}
