package system

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// viewChanges 描述一次事务中需要推进的读视图。每个 scope + resource_id 最多推进一次，
// 因此同一事务内不会重复 bump 同一视图。
//
// 失效范围按 API 实际返回的数据划分，而不是按“写入了什么”：
//   - targets:<service_id>  目标状态 / 连续成功失败次数 / 最近检查时间
//   - service:<service_id>  服务详情自己的字段（聚合状态、元数据）
//   - services             服务列表可见字段（聚合状态、活跃 Episode 概览）
//   - episodes             Episode 创建 / 输入版本 / 状态 / resolution_pending
//   - dashboard            统计与摘要（按状态的计数、最近 Episode 列表）
//
// 纯时间戳抖动（target last_check_at、episode updated_at）以及 Page/列表里的
// last_check_at 不单独触发全局视图失效：读缓存自身有 5s TTL 兜底，且这正是本轮
// 要收敛的写放大来源。确实影响结果的字段变化必须置 true，不能漏。
type viewChanges struct {
	serviceID uuid.UUID
	dashboard bool
	services  bool
	episodes  bool
	service   bool
	targets   bool
}

// noteViewChanges 只推进本次事务中确实发生变化的视图。
func (s *Store) noteViewChanges(ctx context.Context, tx *sql.Tx, ch viewChanges) error {
	if ch.dashboard {
		if err := s.bumpRevision(ctx, tx, "dashboard", uuid.Nil); err != nil {
			return err
		}
	}
	if ch.services {
		if err := s.bumpRevision(ctx, tx, "services", uuid.Nil); err != nil {
			return err
		}
	}
	if ch.episodes {
		if err := s.bumpRevision(ctx, tx, "episodes", uuid.Nil); err != nil {
			return err
		}
	}
	if ch.service {
		if err := s.bumpRevision(ctx, tx, "service", ch.serviceID); err != nil {
			return err
		}
	}
	if ch.targets {
		if err := s.bumpRevision(ctx, tx, "targets", ch.serviceID); err != nil {
			return err
		}
	}
	return nil
}

// noteViews 会在一次事务里推进该服务的全部 5 个视图，用于服务/目标元数据变更
// （创建、Patch、软删除）这类无法按字段精确判断的操作。监控采样与应用事件应按
// 真实变化调用 noteViewChanges，而不是走这里。
func (s *Store) noteViews(ctx context.Context, tx *sql.Tx, serviceID uuid.UUID) error {
	return s.noteViewChanges(ctx, tx, viewChanges{
		serviceID: serviceID,
		dashboard: true,
		services:  true,
		episodes:  true,
		service:   true,
		targets:   true,
	})
}

func (s *Store) bumpRevision(ctx context.Context, tx *sql.Tx, scope string, id uuid.UUID) error {
	var revision int64
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO system.resource_revisions (scope, resource_id, revision)
		VALUES ($1,$2,1)
		ON CONFLICT (scope, resource_id)
		DO UPDATE SET revision = system.resource_revisions.revision + 1
		RETURNING revision`, scope, id).Scan(&revision); err != nil {
		return err
	}
	eventID := uuid.New()
	raw, _ := json.Marshal(map[string]any{
		"scope": scope, "resource_id": id.String(), "revision": revision,
	})
	_, err := tx.ExecContext(ctx, `
		INSERT INTO system.outbox_events
		(outbox_id, event_id, aggregate_type, aggregate_id, event_type, schema_version, payload, next_retry_at, created_at)
		VALUES ($1,$2,'RESOURCE',$3,'RESOURCE_REVISION',1,$4,$5,$5)`,
		uuid.New(), eventID, id, raw, time.Now().UTC())
	return err
}

func (s *Store) PublishByType(ctx context.Context, eventType string, limit int, send func(payload []byte) error) (int, error) {
	if limit <= 0 {
		limit = 16
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `
		SELECT outbox_id, payload FROM system.outbox_events
		WHERE published_at IS NULL AND next_retry_at <= $1 AND event_type=$2
		ORDER BY created_at
		LIMIT $3
		FOR UPDATE SKIP LOCKED`, s.now(), eventType, limit)
	if err != nil {
		return 0, err
	}
	type item struct {
		id      uuid.UUID
		payload []byte
	}
	var batch []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.id, &it.payload); err != nil {
			rows.Close()
			return 0, err
		}
		batch = append(batch, it)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	published := 0
	now := s.now()
	for _, it := range batch {
		if err := send(it.payload); err != nil {
			if _, uerr := tx.ExecContext(ctx, `
				UPDATE system.outbox_events
				SET publish_attempts=publish_attempts+1, next_retry_at=$2
				WHERE outbox_id=$1`, it.id, now.Add(s.RetryBase)); uerr != nil {
				return published, uerr
			}
			continue
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE system.outbox_events SET published_at=$2 WHERE outbox_id=$1`, it.id, now); err != nil {
			return published, err
		}
		published++
	}
	return published, tx.Commit()
}
