package mapper

import (
	"encoding/json"
	"strconv"
	"time"

	appepisode "github.com/ecakeman/alert/internal/application/episode"
	portepisode "github.com/ecakeman/alert/internal/application/port/episode"
	portmonitor "github.com/ecakeman/alert/internal/application/port/monitor"
	porttask "github.com/ecakeman/alert/internal/application/port/task"
	"github.com/ecakeman/alert/internal/transport/http/dto"
)

func fmtTime(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.UTC().Format(time.RFC3339)
	return &s
}

// episodeSummaryCore 是 EpisodeSummary 公共字段的中间载体：
// ToEpisodeSummary（episode 聚合读模型）与 ToRecentEpisodeSummary（dashboard 拍平读模型）
// 的两种输入不强行统一，仅将公共字段填充收敛为唯一实现。
type episodeSummaryCore struct {
	EpisodeID             string
	ServiceID             string
	ServiceName           string
	Status                string
	ResolutionPending     bool
	StartedBy             string
	StartedAt             time.Time
	ResolvedAt            *time.Time
	ArchivedAt            *time.Time
	InputVersion          int64
	ProcessedInputVersion int64
	ActiveTaskStatus      string
	ActiveTaskType        string
	UpdatedAt             time.Time
}

// fillEpisodeSummary 唯一填充 dto.EpisodeSummary 公共字段（含 ActiveTaskType 非空写法）；
// HasDiagnosisReport 为差异字段，由各调用方自行补齐。
func fillEpisodeSummary(c episodeSummaryCore) dto.EpisodeSummary {
	out := dto.EpisodeSummary{
		EpisodeID: c.EpisodeID, ServiceID: c.ServiceID, ServiceName: c.ServiceName,
		Status: c.Status, ResolutionPending: c.ResolutionPending, StartedBy: c.StartedBy,
		StartedAt:  c.StartedAt.UTC().Format(time.RFC3339),
		ResolvedAt: fmtTime(c.ResolvedAt), ArchivedAt: fmtTime(c.ArchivedAt),
		InputVersion: c.InputVersion, ProcessedInputVersion: c.ProcessedInputVersion,
		ActiveTaskStatus: c.ActiveTaskStatus, UpdatedAt: c.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if c.ActiveTaskType != "" {
		at := c.ActiveTaskType
		out.ActiveTaskType = &at
	}
	return out
}

func ToEpisodeView(v appepisode.EpisodeView) dto.EpisodeView {
	monitor, app := "UNKNOWN", "UNKNOWN"
	for _, st := range v.Sources {
		switch st.SourceType {
		case "MONITOR":
			monitor = st.Status
		case "APP":
			app = st.Status
		}
	}
	ep := v.Episode
	name := v.ServiceName
	out := dto.EpisodeView{
		EpisodeID: ep.EpisodeID, ServiceID: ep.ServiceID, ServiceName: &name,
		Status: ep.Status, ResolutionPending: ep.ResolutionPending, StartedBy: ep.StartedBy,
		StartedAt:  ep.StartedAt.UTC().Format(time.RFC3339),
		ResolvedAt: fmtTime(ep.ResolvedAt), ArchivedAt: fmtTime(ep.ArchivedAt),
		InputHash: ep.InputHash, InputVersion: ep.InputVersion,
		ProcessedInputVersion: ep.ProcessedInputVersion, FinalInputVersion: ep.FinalInputVersion,
		MonitorStatus: monitor, AppStatus: app,
	}
	if v.ActiveTask != nil {
		out.ActiveTaskID = v.ActiveTask.TaskID
		out.ActiveTaskStatus = v.ActiveTask.Status
		out.ActiveTaskType = v.ActiveTask.TaskType
	}
	return out
}

func ToEpisodeSummary(d appepisode.EpisodeSummaryData) dto.EpisodeSummary {
	ep := d.Episode
	out := fillEpisodeSummary(episodeSummaryCore{
		EpisodeID: ep.EpisodeID, ServiceID: ep.ServiceID, ServiceName: d.ServiceName,
		Status: ep.Status, ResolutionPending: ep.ResolutionPending, StartedBy: ep.StartedBy,
		StartedAt: ep.StartedAt, ResolvedAt: ep.ResolvedAt, ArchivedAt: ep.ArchivedAt,
		InputVersion: ep.InputVersion, ProcessedInputVersion: ep.ProcessedInputVersion,
		ActiveTaskStatus: d.ActiveTaskStatus, ActiveTaskType: d.ActiveTaskType,
		UpdatedAt: ep.UpdatedAt,
	})
	if d.HasDiagnosisReport {
		has := true
		out.HasDiagnosisReport = &has
	}
	return out
}

func ToMonitorFields(fields []appepisode.MonitorFieldView) []dto.MonitorField {
	out := make([]dto.MonitorField, 0, len(fields))
	for _, f := range fields {
		out = append(out, dto.MonitorField{
			TargetKey: f.TargetKey, TargetType: f.TargetType, Status: f.Status,
			StatusClass: f.StatusClass, ErrorClass: f.ErrorClass, StateFingerprint: f.StateFingerprint,
		})
	}
	return out
}

func ToMonitorPoints(ps []portmonitor.MonitorPoint) []dto.MonitorPoint {
	out := make([]dto.MonitorPoint, 0, len(ps))
	for _, p := range ps {
		out = append(out, dto.MonitorPoint{
			PointSeq: p.PointSeq, TargetID: p.TargetID, TargetKey: p.TargetKey,
			TargetType: p.TargetType, ObservedAt: p.ObservedAt.UTC().Format(time.RFC3339),
			CheckStatus: p.CheckStatus, StatusClass: p.StatusClass, ErrorClass: p.ErrorClass,
		})
	}
	return out
}

func ToAppEvent(e portepisode.AppEvent) dto.AppEvent {
	return dto.AppEvent{
		EventRecordID: strconv.FormatInt(e.EventRecordID, 10), EventID: e.EventID,
		EventKind: e.EventKind, HealthStatus: e.HealthStatus,
		EventTime:  e.EventTime.UTC().Format(time.RFC3339),
		ReceivedAt: e.ReceivedAt.UTC().Format(time.RFC3339),
		Payload:    json.RawMessage(e.Payload),
	}
}

func ToAppEvents(es []portepisode.AppEvent) []dto.AppEvent {
	out := make([]dto.AppEvent, 0, len(es))
	for _, e := range es {
		out = append(out, ToAppEvent(e))
	}
	return out
}

func ToArtifactRefs(as []porttask.ArtifactRef) []dto.ArtifactRef {
	out := make([]dto.ArtifactRef, 0, len(as))
	for _, a := range as {
		out = append(out, dto.ArtifactRef{
			ArtifactType: a.ArtifactType, ArtifactVersion: a.ArtifactVersion,
			StorageKey: a.StorageKey, ContentType: a.ContentType,
			ContentHash: a.ContentHash, UpdatedAt: a.UpdatedAt.UTC().Format(time.RFC3339),
		})
	}
	return out
}
