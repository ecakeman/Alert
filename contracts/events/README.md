# 事件信封契约 v1（语言无关）

本目录是 Go（`go/contracts` 及各域服务）与 Python（`python/agent-runtime`）之间**唯一**的事件线上契约源。
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

## 事件类型（8 个，一格一属）

| event_type | topic | aggregate_type | 发布者 | 订阅者 | payload 要点 |
|---|---|---|---|---|---|
| `service.state.changed.v1` | `alertflow.registry.events.v1` | `service` | registry | monitor | `{ "service_id": "..." }`（语义不变） |
| `service.catalog.changed.v1` | `alertflow.registry.events.v1` | `service` | registry | monitor | 服务目录快照：`service` + `targets[]`（见 `payloads/service.catalog.changed.v1.json`） |
| `monitor.target.state.changed.v1` | `alertflow.monitor.events.v1` | `monitor_target` | monitor | registry | `{service_id,target_id,status,consecutive_failures,consecutive_successes,last_check_at}` |
| `monitor.service.state.changed.v1` | `alertflow.monitor.events.v1` | `service` | monitor | registry、episode | `{service_id,source_status,status,aggregation_mode,target_health_digest}` |
| `episode.status.changed.v1` | `alertflow.episode.events.v1` | `episode` | episode | task（审计） | `{ "episode_id": "...", "status": "..." }`（语义不变） |
| `episode.input.appended.v1` | `alertflow.episode.events.v1` | `episode` | episode | task（审计） | `{ "episode_id": "...", "input_version": N }`（语义不变；**注意**现存两处发布点 payload 形状不同，见下方历史差异） |
| `episode.dispatchable.v1` | `alertflow.episode.events.v1` | `episode` | episode | task | 派发决策快照（11 + 3 字段，见 `payloads/episode.dispatchable.v1.json`） |
| `task.dispatch.requested.v1` | `alertflow.task.events.v1` | `task` | task | task（自身消费） | `{ "task_id": "...", "generation": N }`（语义不变） |

### 历史差异标注（只标注，不修改既有 payload）

`episode.input.appended.v1` 在单体（`go/services/system-core`）中存在两处发布点，payload 形状不同：

| 发布点 | payload 形状 |
|---|---|
| `go/services/system-core/internal/application/episode/episode.go:135` | `{service_id,event_id}` |
| `go/services/system-core/internal/application/episode/episode.go:205` | `{episode_id,input_version}` |

微服务拆分后，**新消费者不解析该事件做主路径**：任务派发只依赖新增的 `episode.dispatchable.v1`，该既有事件仅用于审计。既有 payload 保持不变（符合 spec 4.5.2 兼容规则）。

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

- **Go**：唯一实现位于 `go/contracts`（module `github.com/ecakeman/alert/services/contracts`），负责构造、编码、发布、解码，并以此契约生成线上 JSON。`go/services/system-core/internal/contracts` 为过渡期旧副本，P6 收口时删除。
- **Python**：位于 `python/agent-runtime`，应基于本 Schema 生成/校验消费模型。
- 本文件只描述契约，**不得**回写成任何一侧的内部模型或数据库行。