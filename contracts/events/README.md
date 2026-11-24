# 事件信封契约 v1（语言无关）

本目录是 Go（`services/system-core`）与未来 Python（`services/agent-runtime`）之间**唯一**的事件线上契约源。
本文件与 `event-envelope.v1.schema.json` 只描述**公开字段**；投递控制字段（claim/ack/重试）不属于线上契约。

## 字段（8 个）

| 字段 | 必填 | 说明 |
|---|---|---|
| `event_id` | 是 | 事件唯一 ID（UUID） |
| `event_type` | 是 | 事件类型，见下表 |
| `schema_version` | 是 | 正整数，当前为 `1` |
| `aggregate_type` | 是 | 聚合类型（如 `service`、`episode`） |
| `aggregate_id` | 是 | 聚合 ID |
| `occurred_at` | 是 | UTC RFC3339 |
| `trace_id` | 否 | 分布式追踪 ID（可空） |
| `payload` | 是 | 业务载荷（JSON object） |

**禁止**在线上信封中出现投递控制字段：`claim_owner`、`claim_until`、`publish_attempts`、`next_retry_at`、`published_at`、`last_error`。这些只存在于 Outbox 存储行（`port.OutboxRecord`），不进入本契约。

## 事件类型（当前 3 个）

| event_type | aggregate_type | payload 要点 |
|---|---|---|
| `service.state.changed.v1` | `service` | `{ "service_id": "..." }` |
| `episode.status.changed.v1` | `episode` | `{ "episode_id": "...", "status": "..." }` |
| `episode.input.appended.v1` | `episode` | `{ "episode_id": "...", "input_version": N }` |

## 兼容规则

1. **只增可选字段**；已发布字段不得改名、改类型、改语义。
2. 破坏性变更必须递增 `schema_version`，并新增 `event-envelope.v2.schema.json`，保留 v1 供旧消费者。
3. 消费者对未知 `event_type` / `schema_version` 必须**显式失败**，不得静默标记完成。

## 示例

```json
{
  "event_id": "c38a23d9-0016-4239-a5ab-171ab9b85f95",
  "event_type": "service.state.changed.v1",
  "schema_version": 1,
  "aggregate_type": "service",
  "aggregate_id": "47ceb5ca-3235-43c0-9089-53d7bdf412ed",
  "occurred_at": "2026-11-23T14:42:17Z",
  "payload": { "service_id": "47ceb5ca-3235-43c0-9089-53d7bdf412ed" }
}
```

```json
{
  "event_id": "11111111-1111-1111-1111-111111111111",
  "event_type": "episode.input.appended.v1",
  "schema_version": 1,
  "aggregate_type": "episode",
  "aggregate_id": "22222222-2222-2222-2222-222222222222",
  "occurred_at": "2026-11-23T14:50:00Z",
  "trace_id": "trace-abc",
  "payload": { "episode_id": "22222222-2222-2222-2222-222222222222", "input_version": 2 }
}
```

## 归属

- **Go**：类型位于 `services/system-core/internal/contracts`（S3 搬迁前为根 `internal/contracts`），负责构造、编码、发布、解码，并以此契约生成线上 JSON。
- **Python**：未来位于 `services/agent-runtime`，应基于本 Schema 生成/校验消费模型。
- 本文件只描述契约，**不得**回写成任何一侧的内部模型或数据库行。