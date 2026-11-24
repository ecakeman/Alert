package episode

import (
	"context"
	"encoding/json"
	"sort"
	"strconv"
	"time"

	"github.com/ecakeman/alert/internal/application/apperr"
	"github.com/ecakeman/alert/internal/application/port"
)

type TimelineData struct {
	Episode         EpisodeView
	Projection      []MonitorFieldView
	MonitorStartSeq int64
	MonitorPoints   []port.MonitorPoint
	AppEvents       []port.AppEvent
	Artifacts       []port.ArtifactRef
}

type ServiceTimelineRecord struct {
	RecordType string
	RecordID   string
	EventTime  time.Time
	ReceivedAt time.Time
	Source     string
	Content    []byte
}

// ServiceTimeline 返回服务级时间轴（监控点 + APP 事件，按发生时间升序）。
func (s *Service) ServiceTimeline(ctx context.Context, serviceID string, limit int) ([]ServiceTimelineRecord, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var records []ServiceTimelineRecord
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx port.Tx) error {
		latestSeq, err := s.deps.MonitorPoints.LatestSeqByService(ctx, tx, serviceID)
		if err != nil {
			return err
		}
		points, err := s.deps.MonitorPoints.ListByBound(ctx, tx, serviceID, 0, latestSeq, limit)
		if err != nil {
			return err
		}
		events, err := s.deps.AppEvents.ListByBound(ctx, tx, serviceID, 0, int64(1)<<62, limit)
		if err != nil {
			return err
		}

		for _, p := range points {
			content, _ := json.Marshal(map[string]string{
				"check_status": p.CheckStatus, "status_class": p.StatusClass,
				"error_class": p.ErrorClass, "state_fingerprint": p.StateFingerprint,
			})
			records = append(records, ServiceTimelineRecord{
				RecordType: "MONITOR_POINT", RecordID: strconv.FormatInt(p.PointSeq, 10),
				EventTime: p.ObservedAt, ReceivedAt: p.ReceivedAt, Source: "MONITOR", Content: content,
			})
		}
		for _, e := range events {
			records = append(records, ServiceTimelineRecord{
				RecordType: "APP_EVENT", RecordID: strconv.FormatInt(e.EventRecordID, 10),
				EventTime: e.EventTime, ReceivedAt: e.ReceivedAt, Source: "APP", Content: e.Payload,
			})
		}
		sort.Slice(records, func(i, j int) bool { return records[i].EventTime.Before(records[j].EventTime) })
		if len(records) > limit {
			records = records[len(records)-limit:]
		}
		return nil
	})
	return records, err
}

// Timeline 返回故障级时间轴，受任务冻结边界（若存在活动任务）约束。
func (s *Service) Timeline(ctx context.Context, episodeID string) (TimelineData, error) {
	var d TimelineData
	err := s.deps.UOW.WithinTx(ctx, func(ctx context.Context, tx port.Tx) error {
		ep, err := s.deps.Episodes.Get(ctx, tx, episodeID)
		if err != nil {
			return err
		}
		sources, err := s.deps.Sources.ListByService(ctx, tx, ep.ServiceID)
		if err != nil {
			return err
		}
		svc, err := s.deps.Services.Get(ctx, tx, ep.ServiceID)
		if err != nil {
			return err
		}
		tasks, err := s.deps.Tasks.ListActiveByEpisode(ctx, tx, episodeID)
		if err != nil {
			return err
		}

		view := EpisodeView{Episode: ep, Sources: sources, ServiceName: svc.Name}
		if len(tasks) > 0 {
			view.ActiveTask = &tasks[0]
		}

		toSeq, toEvent := ep.MonitorWatermark, ep.AppEventWatermark
		if view.ActiveTask != nil {
			toSeq, toEvent = view.ActiveTask.MonitorWatermark, view.ActiveTask.AppEventWatermark
		}

		var projection []MonitorFieldView
		latest, err := s.deps.InputVersions.GetLatest(ctx, tx, episodeID)
		if err == nil {
			if len(latest.MonitorProjection) > 0 {
				_ = json.Unmarshal(latest.MonitorProjection, &projection)
			}
			d.MonitorStartSeq = latest.MonitorWatermark
		} else if !isAppCode(err, apperr.CodeNotFound) {
			return err
		}

		points, err := s.deps.MonitorPoints.ListByBound(ctx, tx, ep.ServiceID, 0, toSeq, 500)
		if err != nil {
			return err
		}
		events, err := s.deps.AppEvents.ListByBound(ctx, tx, ep.ServiceID, 0, toEvent, 500)
		if err != nil {
			return err
		}
		artifacts, err := s.deps.Artifacts.ListByEpisode(ctx, tx, episodeID)
		if err != nil {
			return err
		}

		d.Episode = view
		d.Projection = projection
		d.MonitorPoints = points
		d.AppEvents = events
		d.Artifacts = artifacts
		return nil
	})
	return d, err
}
