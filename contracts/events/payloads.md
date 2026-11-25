# 事件 payload 字段清单（v1）

本文件是 8 类事件 `payload` 的字段级说明，信封字段见 `README.md`。
示例 JSON 位于 `payloads/`，仅描述 `payload` 对象本身，不含信封。

## 既有 4 类（payload 语义不变）

### service.state.changed.v1

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `service_id` | string(UUID) | 是 | 服务 ID |

### episode.status.changed.v1

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `episode_id` | string(UUID) | 是 | 立案 ID |
| `status` | string | 是 | `ACTIVE` / `RESOLVED` |

### episode.input.appended.v1

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `episode_id` | string(UUID) | 是 | 立案 ID |
| `input_version` | integer | 是 | 追加后的输入版本 |

历史差异见 `README.md` 的“历史差异标注”；该事件拆分后仅用于审计。

### task.dispatch.requested.v1

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `task_id` | string(UUID) | 是 | 任务 ID |
| `generation` | integer | 是 | 任务代次（围栏） |

## 新增 4 类

### service.catalog.changed.v1

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `service.id` | string(UUID) | 是 | 服务 ID |
| `service.enabled` | boolean | 是 | 是否启用 |
| `service.aggregation_mode` | string | 是 | 聚合模式 |
| `targets[].target_id` | string(UUID) | 是 | 目标 ID |
| `targets[].target_key` | string | 是 | 目标稳定键 |
| `targets[].target_type` | string | 是 | 目标类型 |
| `targets[].address` | string | 是 | 探测地址 |
| `targets[].interval_ms` | integer | 是 | 探测间隔 |
| `targets[].timeout_ms` | integer | 是 | 探测超时 |
| `targets[].failure_threshold` | integer | 是 | 判异常阈值 |
| `targets[].recovery_threshold` | integer | 是 | 判恢复阈值 |
| `targets[].is_required` | boolean | 是 | 是否必需目标 |
| `targets[].enabled` | boolean | 是 | 是否启用 |
| `targets[].config_generation` | integer | 是 | 配置代次（增量同步用） |

### monitor.target.state.changed.v1

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `service_id` | string(UUID) | 是 | 服务 ID |
| `target_id` | string(UUID) | 是 | 目标 ID |
| `status` | string | 是 | 目标状态 |
| `consecutive_failures` | integer | 是 | 连续失败次数 |
| `consecutive_successes` | integer | 是 | 连续成功次数 |
| `last_check_at` | string(RFC3339) | 是 | 最近探测时刻 |

### monitor.service.state.changed.v1

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `service_id` | string(UUID) | 是 | 服务 ID |
| `source_status` | string | 是 | 变化前来源状态 |
| `status` | string | 是 | 变化后来源状态 |
| `aggregation_mode` | string | 是 | 聚合模式 |
| `target_health_digest` | string | 是 | 目标健康摘要（变化判定） |

### episode.dispatchable.v1

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `episode_id` | string(UUID) | 是 | 立案 ID |
| `service_id` | string(UUID) | 是 | 服务 ID |
| `episode_status` | string | 是 | 立案状态 |
| `input_version_from` | integer | 是 | 冻结输入区间起点 |
| `input_version_to` | integer | 是 | 冻结输入区间终点 |
| `monitor_watermark` | integer | 是 | 监控水位线 |
| `app_event_watermark` | integer | 是 | 应用事件水位线 |
| `final_input_version` | integer | 是 | 最终输入版本 |
| `processed_input_version` | integer | 是 | 已处理输入版本 |
| `reason` | string | 是 | `INPUT_APPENDED` / `EPISODE_RESOLVED` |
| `artifact_readiness.investigation` | boolean | 是 | 调查产物是否齐全 |
| `artifact_readiness.diagnosis_report` | boolean | 是 | 诊断报告是否齐全 |
| `artifact_readiness.hashes_match` | boolean | 是 | 产物 hash 是否一致 |

消费纪律：task 侧以 `episode:<episode_id>:<input_version_to>:<reason>` 作为附加幂等键，重复投递不重复建任务。