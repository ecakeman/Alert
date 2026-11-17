package system

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

// revisionSnapshot 记录某一时刻的 revision 指针与已写入的 RESOURCE_REVISION outbox 条数。
// 断言一律使用「单次增量」：testStore 不清理 system.resource_revisions，全局累计行数会跨用例漂移。
type revisionSnapshot struct {
	revisions map[string]int64
	outbox    int64
}

func snapshotRevisions(t *testing.T, s *Store) revisionSnapshot {
	t.Helper()
	snap := revisionSnapshot{revisions: map[string]int64{}}
	rows, err := s.DB.Query(`SELECT scope, resource_id, revision FROM system.resource_revisions`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var scope string
		var resourceID uuid.UUID
		var revision int64
		if err := rows.Scan(&scope, &resourceID, &revision); err != nil {
			t.Fatal(err)
		}
		snap.revisions[viewKey(scope, resourceID)] = revision
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM system.outbox_events WHERE event_type='RESOURCE_REVISION'`).Scan(&snap.outbox); err != nil {
		t.Fatal(err)
	}
	return snap
}

func viewKey(scope string, resourceID uuid.UUID) string {
	return scope + ":" + resourceID.String()
}

func changedViews(before, after revisionSnapshot) map[string]int64 {
	changed := map[string]int64{}
	for key, revision := range after.revisions {
		prev, seen := before.revisions[key]
		if !seen || revision != prev {
			changed[key] = revision - prev
		}
	}
	return changed
}

// assertRevisionBumps 断言本次事务只推进了 wantViews 中各一次，且 outbox 的 RESOURCE_REVISION
// 增量恰好等于推进的视图数。视图的增量必须为 +1：同事务同 scope+id 重复 bump 会让增量 >1。
func assertRevisionBumps(t *testing.T, before, after revisionSnapshot, wantViews ...string) {
	t.Helper()
	want := make(map[string]bool, len(wantViews))
	for _, key := range wantViews {
		want[key] = true
	}
	changed := changedViews(before, after)
	for key, delta := range changed {
		if !want[key] {
			t.Errorf("unexpected revision bump: %s (+%d)", key, delta)
		}
		if delta != 1 {
			t.Errorf("revision %s bumped %d times in one transaction, want 1", key, delta)
		}
	}
	for key := range before.revisions {
		if _, ok := after.revisions[key]; !ok {
			t.Errorf("revision row disappeared: %s", key)
		}
	}
	for key := range want {
		if _, ok := changed[key]; !ok {
			t.Errorf("missing revision bump: %s", key)
		}
	}
	if got := after.outbox - before.outbox; got != int64(len(want)) {
		t.Errorf("RESOURCE_REVISION outbox delta = %d, want %d", got, len(want))
	}
}

func assertNoRevisionBumps(t *testing.T, before, after revisionSnapshot) {
	t.Helper()
	assertRevisionBumps(t, before, after)
}

// assertBumpInvariant 只校验结构不变量：outbox 增量 == 实际变化的视图数，且每个视图只 bump 一次。
// 不预设具体视图，用于验证「写入数量 == 实际变化视图数」本身。
func assertBumpInvariant(t *testing.T, before, after revisionSnapshot) {
	t.Helper()
	changed := changedViews(before, after)
	for key, delta := range changed {
		if delta != 1 {
			t.Errorf("revision %s bumped %d times in one transaction, want 1", key, delta)
		}
	}
	if got := int64(len(changed)); after.outbox-before.outbox != got {
		t.Errorf("RESOURCE_REVISION outbox delta = %d, changed views = %d", after.outbox-before.outbox, got)
	}
}

// finishActiveTask 完成 Episode 当前的活跃任务；诊断任务需要给出处理的输入版本，归档任务需要 fd_id。
func finishActiveTask(t *testing.T, s *Store, episodeID uuid.UUID, wantType string, processedInputVersion int64, fdID string) {
	t.Helper()
	ctx := context.Background()
	view, err := s.GetEpisode(ctx, episodeID)
	if err != nil {
		t.Fatal(err)
	}
	if view.ActiveTaskType != wantType || view.ActiveTaskID == nil {
		t.Fatalf("active task = %q %v, want %q", view.ActiveTaskType, view.ActiveTaskID, wantType)
	}
	var to, generation int64
	if err := s.DB.QueryRow(`
		SELECT COALESCE(input_version_to, 0), execution_generation
		FROM system.agent_tasks WHERE task_id=$1`, *view.ActiveTaskID).Scan(&to, &generation); err != nil {
		t.Fatal(err)
	}
	if wantType == TaskDiagnosis && to != processedInputVersion {
		t.Fatalf("diagnosis input_version_to = %d, want %d", to, processedInputVersion)
	}
	ack, err := s.CompleteTask(ctx, CompleteInput{
		TaskID: *view.ActiveTaskID, EpisodeID: episodeID, TaskType: wantType, Generation: generation,
		Outcome: "SUCCEEDED", ProcessedInputVersion: processedInputVersion, FDID: fdID,
	})
	if err != nil || !ack.Accepted {
		t.Fatalf("complete %s: %+v %v", wantType, ack, err)
	}
}

// 场景 1：没有 Episode 的普通采样（首次失败但未达阈值）只应推进 targets 视图。
func TestRevisionOrdinarySampleWithoutEpisode(t *testing.T) {
	s := testStore(t)
	svc, target := newSvc(t, s, 2, 2)
	ctx := context.Background()
	base := time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC)

	before := snapshotRevisions(t, s)
	res, err := s.ApplyMonitorSample(ctx, sample(svc, target, false, "5xx", base))
	if err != nil {
		t.Fatal(err)
	}
	if res.CreatedEpisode || res.EpisodeID != uuid.Nil {
		t.Fatalf("below-threshold failure must not open an episode: %+v", res)
	}
	if res.MonitorStatus != StatusUnknown {
		t.Fatalf("aggregation must stay UNKNOWN, got %s", res.MonitorStatus)
	}
	assertRevisionBumps(t, before, snapshotRevisions(t, s), viewKey("targets", svc))
}

// 场景 2：已有 Episode 的普通采样，Episode 可见字段没有变化，只推进 targets 视图。
func TestRevisionOrdinarySampleWithEpisode(t *testing.T) {
	s := testStore(t)
	svc, target := newSvc(t, s, 1, 1)
	ctx := context.Background()
	base := time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC)

	opened, err := s.ApplyMonitorSample(ctx, sample(svc, target, false, "5xx", base))
	if err != nil || !opened.CreatedEpisode {
		t.Fatalf("open episode: %+v %v", opened, err)
	}

	// 同样的 5xx 重复失败：目标状态与聚合状态都没变，输入哈希不变。
	before := snapshotRevisions(t, s)
	repeat, err := s.ApplyMonitorSample(ctx, sample(svc, target, false, "5xx", base.Add(time.Second)))
	if err != nil {
		t.Fatal(err)
	}
	if repeat.CreatedEpisode || repeat.EpisodeID != opened.EpisodeID || repeat.InputVersion != opened.InputVersion {
		t.Fatalf("repeat failure must not change episode: %+v", repeat)
	}
	if repeat.MonitorStatus != StatusAbnormal {
		t.Fatalf("monitor = %s, want ABNORMAL", repeat.MonitorStatus)
	}
	assertRevisionBumps(t, before, snapshotRevisions(t, s), viewKey("targets", svc))
}

// 场景 3：NORMAL/UNKNOWN → ABNORMAL 首次创建 Episode，service/services/dashboard/episodes 都需要失效。
func TestRevisionFirstAbnormalCreatesEpisode(t *testing.T) {
	s := testStore(t)
	svc, target := newSvc(t, s, 1, 1)
	ctx := context.Background()
	base := time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC)

	before := snapshotRevisions(t, s)
	opened, err := s.ApplyMonitorSample(ctx, sample(svc, target, false, "5xx", base))
	if err != nil {
		t.Fatal(err)
	}
	if !opened.CreatedEpisode || opened.MonitorStatus != StatusAbnormal || opened.InputVersion != 1 {
		t.Fatalf("open episode: %+v", opened)
	}
	assertRevisionBumps(t, before, snapshotRevisions(t, s),
		viewKey("targets", svc),
		viewKey("service", svc),
		viewKey("services", uuid.Nil),
		viewKey("episodes", uuid.Nil),
		viewKey("dashboard", uuid.Nil))
}

// 场景 4：恢复 → 解析（RESOLVED）→ 归档（ARCHIVED）沿路的失效范围。
func TestRevisionRecoveryAndArchive(t *testing.T) {
	s := testStore(t)
	svc, target := newSvc(t, s, 1, 1)
	ctx := context.Background()
	base := time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC)

	// 1. 首次异常采样：创建 Episode，5 个视图各推进一次。
	before := snapshotRevisions(t, s)
	opened, err := s.ApplyMonitorSample(ctx, sample(svc, target, false, "5xx", base))
	if err != nil {
		t.Fatal(err)
	}
	assertRevisionBumps(t, before, snapshotRevisions(t, s),
		viewKey("targets", svc),
		viewKey("service", svc),
		viewKey("services", uuid.Nil),
		viewKey("episodes", uuid.Nil),
		viewKey("dashboard", uuid.Nil))

	// 2. APP 上报 NORMAL：app 状态 UNKNOWN→NORMAL，输入版本 1→2。
	revision := int64(1)
	raw, ev := appBody("n1", "HEALTH_STATUS", StatusNormal, &revision)
	before = snapshotRevisions(t, s)
	appRes, err := s.ApplyAppEvent(ctx, svc, ev, raw)
	if err != nil {
		t.Fatal(err)
	}
	if appRes.Status != 200 || appRes.Body["input_version"] != int64(2) {
		t.Fatalf("app normal: %+v", appRes)
	}
	assertRevisionBumps(t, before, snapshotRevisions(t, s),
		viewKey("service", svc),
		viewKey("services", uuid.Nil),
		viewKey("episodes", uuid.Nil),
		viewKey("dashboard", uuid.Nil))

	// 3. 处理完两个诊断任务，Episode 仍为 ACTIVE：不产生任何 revision / outbox 写入。
	for _, to := range []int64{1, 2} {
		before = snapshotRevisions(t, s)
		finishActiveTask(t, s, opened.EpisodeID, TaskDiagnosis, to, "")
		assertNoRevisionBumps(t, before, snapshotRevisions(t, s))
	}

	// 4. 恢复采样：MONITOR ABNORMAL→NORMAL，输入版本 2→3，进入 resolution_pending。
	before = snapshotRevisions(t, s)
	recovered, err := s.ApplyMonitorSample(ctx, sample(svc, target, true, "2xx", base.Add(3*time.Minute)))
	if err != nil {
		t.Fatal(err)
	}
	if recovered.MonitorStatus != StatusNormal || recovered.InputVersion != 3 {
		t.Fatalf("recovery: %+v", recovered)
	}
	assertRevisionBumps(t, before, snapshotRevisions(t, s),
		viewKey("targets", svc),
		viewKey("service", svc),
		viewKey("services", uuid.Nil),
		viewKey("episodes", uuid.Nil),
		viewKey("dashboard", uuid.Nil))

	// 5. 处理完最终版本：Episode ACTIVE→RESOLVED，services 的活跃 Episode 概览也变了。
	before = snapshotRevisions(t, s)
	finishActiveTask(t, s, opened.EpisodeID, TaskDiagnosis, 3, "")
	resolved, err := s.GetEpisode(ctx, opened.EpisodeID)
	if err != nil || resolved.Status != EpisodeResolved || resolved.ActiveTaskType != TaskArchive {
		t.Fatalf("resolved: %+v %v", resolved, err)
	}
	assertRevisionBumps(t, before, snapshotRevisions(t, s),
		viewKey("services", uuid.Nil),
		viewKey("episodes", uuid.Nil),
		viewKey("dashboard", uuid.Nil))

	// 6. 归档 RESOLVED→ARCHIVED：episodes 与 dashboard 失效，services 的活跃 Episode 概览不变。
	fdID := uuid.New().String()
	hash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	registered, message, err := s.RegisterFaultDocument(ctx, RegisterFaultDocumentInput{
		TaskID: *resolved.ActiveTaskID, EpisodeID: resolved.EpisodeID, Generation: 1,
		FDID: fdID, ContentHash: hash,
	})
	if err != nil || !registered || message != "registered" {
		t.Fatalf("register fault document: %v %s %v", registered, message, err)
	}
	before = snapshotRevisions(t, s)
	finishActiveTask(t, s, opened.EpisodeID, TaskArchive, 0, fdID)
	archived, err := s.GetEpisode(ctx, opened.EpisodeID)
	if err != nil || archived.Status != EpisodeArchived {
		t.Fatalf("archived: %+v %v", archived, err)
	}
	assertRevisionBumps(t, before, snapshotRevisions(t, s),
		viewKey("episodes", uuid.Nil),
		viewKey("dashboard", uuid.Nil))
}

// 场景 5：单次事务的 revision / outbox 写入数量必须等于实际变化视图数（不预设具体视图）。
func TestRevisionOutboxMatchesChangedViews(t *testing.T) {
	cases := []struct {
		name string
		run  func(t *testing.T, s *Store)
	}{
		{"sample_without_episode", func(t *testing.T, s *Store) {
			svc, target := newSvc(t, s, 2, 2)
			before := snapshotRevisions(t, s)
			if _, err := s.ApplyMonitorSample(context.Background(), sample(svc, target, false, "5xx", time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC))); err != nil {
				t.Fatal(err)
			}
			assertBumpInvariant(t, before, snapshotRevisions(t, s))
		}},
		{"sample_with_episode", func(t *testing.T, s *Store) {
			svc, target := newSvc(t, s, 1, 1)
			ctx := context.Background()
			base := time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC)
			if _, err := s.ApplyMonitorSample(ctx, sample(svc, target, false, "5xx", base)); err != nil {
				t.Fatal(err)
			}
			before := snapshotRevisions(t, s)
			if _, err := s.ApplyMonitorSample(ctx, sample(svc, target, false, "5xx", base.Add(time.Second))); err != nil {
				t.Fatal(err)
			}
			assertBumpInvariant(t, before, snapshotRevisions(t, s))
		}},
		{"first_abnormal_creates_episode", func(t *testing.T, s *Store) {
			svc, target := newSvc(t, s, 1, 1)
			before := snapshotRevisions(t, s)
			if _, err := s.ApplyMonitorSample(context.Background(), sample(svc, target, false, "5xx", time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC))); err != nil {
				t.Fatal(err)
			}
			assertBumpInvariant(t, before, snapshotRevisions(t, s))
		}},
		{"recovery_and_archive", func(t *testing.T, s *Store) {
			svc, target := newSvc(t, s, 1, 1)
			ctx := context.Background()
			base := time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC)
			opened, err := s.ApplyMonitorSample(ctx, sample(svc, target, false, "5xx", base))
			if err != nil {
				t.Fatal(err)
			}
			revision := int64(1)
			raw, ev := appBody("n1", "HEALTH_STATUS", StatusNormal, &revision)
			if _, err := s.ApplyAppEvent(ctx, svc, ev, raw); err != nil {
				t.Fatal(err)
			}
			finishActiveTask(t, s, opened.EpisodeID, TaskDiagnosis, 1, "")
			finishActiveTask(t, s, opened.EpisodeID, TaskDiagnosis, 2, "")
			before := snapshotRevisions(t, s)
			if _, err := s.ApplyMonitorSample(ctx, sample(svc, target, true, "2xx", base.Add(3*time.Minute))); err != nil {
				t.Fatal(err)
			}
			assertBumpInvariant(t, before, snapshotRevisions(t, s))
			before = snapshotRevisions(t, s)
			finishActiveTask(t, s, opened.EpisodeID, TaskDiagnosis, 3, "")
			assertBumpInvariant(t, before, snapshotRevisions(t, s))
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.run(t, testStore(t))
		})
	}
}
