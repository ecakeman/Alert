package mapper

import (
	"encoding/json"

	portService "github.com/ecakeman/alert/services/system-core/internal/application/port/service"
	appservice "github.com/ecakeman/alert/services/system-core/internal/application/service"
	"github.com/ecakeman/alert/services/system-core/internal/transport/http/dto"
)

func ToTarget(t portService.MonitorTarget) dto.Target {
	cfg := json.RawMessage("null")
	if len(t.Config) > 0 {
		cfg = json.RawMessage(t.Config)
	}
	return dto.Target{
		TargetID: t.TargetID, TargetKey: t.TargetKey, TargetName: t.TargetName,
		TargetType: t.TargetType, Address: t.Address, Config: cfg,
		IntervalMS: t.IntervalMS, TimeoutMS: t.TimeoutMS,
		FailureThreshold: t.FailureThreshold, RecoveryThreshold: t.RecoveryThreshold,
		IsRequired: t.IsRequired, Enabled: t.Enabled, Status: t.Status,
		ConsecutiveFailures: t.ConsecutiveFailures, ConsecutiveSuccesses: t.ConsecutiveSuccesses,
		LastCheckAt: fmtTime(t.LastCheckAt),
	}
}

func ToTargetInput(t dto.TargetInput) appservice.TargetInput {
	var cfg []byte
	if len(t.Config) > 0 {
		cfg = []byte(t.Config)
	}
	return appservice.TargetInput{
		TargetKey: t.TargetKey, TargetName: t.TargetName, Address: t.Address,
		TargetType: t.TargetType, Config: cfg, IntervalMS: t.IntervalMS, TimeoutMS: t.TimeoutMS,
		FailureThreshold: t.FailureThreshold, RecoveryThreshold: t.RecoveryThreshold,
		IsRequired: t.IsRequired,
	}
}

func ToTargetPatch(p dto.TargetPatchPayload) portService.TargetPatch {
	var cfg []byte
	if p.Config != nil {
		cfg = []byte(*p.Config)
	}
	return portService.TargetPatch{
		TargetName: p.TargetName, Address: p.Address, Config: cfg,
		IntervalMS: p.IntervalMS, TimeoutMS: p.TimeoutMS,
		FailureThreshold: p.FailureThreshold, RecoveryThreshold: p.RecoveryThreshold,
		IsRequired: p.IsRequired, Enabled: p.Enabled,
	}
}
