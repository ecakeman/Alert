package mapper

import (
	appservice "github.com/ecakeman/alert/services/system-core/internal/application/service"
	"github.com/ecakeman/alert/services/system-core/internal/transport/http/dto"
)

func ToServiceSummary(s appservice.ServiceSummaryData) dto.ServiceSummary {
	return dto.ServiceSummary{
		ServiceID: s.Service.ServiceID, ServiceKey: s.Service.ServiceKey, Name: s.Service.Name,
		Description: s.Service.Description, Enabled: s.Service.Enabled,
		MonitorStatus: s.MonitorStatus, AppStatus: s.AppStatus, TargetCount: s.TargetCount,
		LastCheckAt: fmtTime(s.LastCheckAt), ActiveEpisodeID: s.ActiveEpisodeID,
	}
}

func ToServiceView(v appservice.ServiceViewData) dto.ServiceView {
	targets := make([]dto.Target, 0, len(v.Targets))
	for _, t := range v.Targets {
		targets = append(targets, ToTarget(t))
	}
	s := v.Summary
	return dto.ServiceView{
		ServiceID: s.Service.ServiceID, ServiceKey: s.Service.ServiceKey, Name: s.Service.Name,
		Description: s.Service.Description, Enabled: s.Service.Enabled,
		MonitorAggregationMode: s.Service.MonitorAggregationMode,
		AuthorityProducerID:    s.Service.AuthorityProducerID,
		MonitorStatus:          s.MonitorStatus, AppStatus: s.AppStatus, Targets: targets,
		TargetCount: s.TargetCount, LastCheckAt: fmtTime(s.LastCheckAt), ActiveEpisodeID: s.ActiveEpisodeID,
	}
}
