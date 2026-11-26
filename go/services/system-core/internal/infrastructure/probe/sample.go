package probe

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	portService "github.com/ecakeman/alert/services/system-core/internal/application/port/service"
)

// Sample 是单次探测结果（不含目标身份，由引擎补齐）。
type Sample struct {
	CheckStatus      string
	StatusClass      string
	ErrorClass       string
	LatencyMS        *int
	StateFingerprint string
}

// Prober 执行一次真实探测。
type Prober interface {
	Probe(ctx context.Context, target portService.MonitorTarget) Sample
}

func fingerprint(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func latencyMS(ms int) *int { return &ms }
