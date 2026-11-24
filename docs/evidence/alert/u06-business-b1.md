# U06 B1 数据层与迁移 — 证据

- 日期：2026-11-23
- 工程根：`/home/sancho/workspace/projects/Alert`
- 真实依赖：`postgres:16`（容器 `alert-pg`）healthy；Go 1.27.4
- 原则：零降级；只在真实 PG 上判定迁移与约束。

## 1. 目标与范围

建出 `system` schema 的 13 张业务表 + 1 张版本表（合计 14），用真实迁移回放与 psql 约束复现证明“约束即不变量”。

**本块不做**：仓储实现（B2）、Handler（B4）、`cognitive`/`gateway`/`resource_revisions`/`application_event_receipts`。

## 2. 迁移文件与表

| 版本 | 文件 | 表 |
|---|---|---|
| 0001 | `0001_system_core_entities.sql` | services、service_monitor_targets、service_source_states、episodes |
| 0002 | `0002_input_events_points.sql` | episode_input_versions、service_monitor_points、episode_app_events |
| 0003 | `0003_tasks_outbox_artifacts.sql` | agent_tasks、service_execution_ownership、outbox_events、consumer_idempotency、episode_artifact_refs、episode_fault_document_refs |
| — | runner 引导 | system.schema_migrations（版本表） |

合计 **14 张**（13 业务 + 版本表）。

## 3. 命令矩阵与三态

| 检查 | 命令 | 结果 |
|---|---|---|
| 静态（排除注释） | grep 检索 DROP/BEGIN/COMMIT/CREATE SCHEMA | 通过（NONE） |
| 首次迁移 | `-migrate`（alert_migrate_check） | 通过（applied=3 skipped=0 conflicts=0，exit 0） |
| 二次迁移 | `-migrate` 再跑 | 通过（applied=0 skipped=3，no-op，exit 0） |
| 14 表 | `information_schema.tables where table_schema='system'` | 通过（14） |
| 篡改检测 | 改 0001 后 `-migrate` | 通过（conflicts 版本 1，exit 1） |
| 还原 | 还原 0001 后 `-migrate` | 通过（skipped=3，exit 0） |
| 新空库回放 | 新建 alert_migrate_check2 后 `-migrate` | 通过（applied=3，exit 0，14 表） |

## 4. 约束真实性复现（psql，9/9 触发）

| 编号 | 不变量 | 期望约束 | 实际 | 结果 |
|---|---|---|---|---|
| T1 | INV-01 单 ACTIVE | `episodes_one_active` | duplicate key | PASS |
| T2 | INV-02 值域 | `source_states_status_check` | check violation | PASS |
| T3 | INV-03 时间一致 | `episodes_resolved_consistency_check` | check violation | PASS |
| T4 | INV-04 指纹唯一 | `episode_input_versions_hash` | duplicate key | PASS |
| T5 | INV-05 事件身份 | `episode_app_events_identity` | duplicate key | PASS |
| T6 | INV-12 活动任务唯一 | `agent_tasks_one_active` | duplicate key | PASS |
| T6b | 任务版本范围 | `tasks_version_range_check` | check violation | PASS |
| T7 | INV-07 所有权唯一 | `service_execution_ownership_pkey` | duplicate key | PASS |
| T8 | INV-11 产物版本唯一 | `episode_artifact_refs_version` | duplicate key | PASS |

`OVERALL_FAIL=0`。

## 5. 路由状态（B1 阶段仍全部 501）

18 条 `/api/v1` 路由保持 `501 NOT_IMPLEMENTED`，未提前实现任何业务。

## 6. 未执行项与已知偏差

- **DROP 被安全策略拦截**：B1.4 原计划 `DROP DATABASE IF EXISTS alert_migrate_check` 重建；实际改用“首次创建空库” (`alert_migrate_check`、`alert_migrate_check2`) 证明空库可回放，未对库名做破坏性删除。属安全策略下的等价验证，不影响结论。
- 全量/E2E 未执行（展示级最小集）。
- 迁移文件命名/约束名已固定，供 B2 按名翻译错误。
- 本地保留 `alert_test`、`alert_migrate_check`、`alert_migrate_check2` 三库，未清理。

## 7. 下一块入口

**AB-B2.1**：端口定义 + UoW + 错误翻译（`internal/application/port`、`postgres/{uow,errors,time}.go`），依赖 B1 的固定约束名。

## 8. 阻塞项

- 无。PG 可用，迁移与约束均已真实验过。