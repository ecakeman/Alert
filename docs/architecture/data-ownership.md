# 数据归属

每个域独占 schema 与数据库角色，禁止跨域直连对方表；跨域只经 L2 API 或事件。

| 域 | schema | role | 迁移位置 | 校验 |
|---|---|---|---|---|
| 目录 | `registry` | `alert_registry` | `go/services/service-registry/migrations/` | `service-registry -migrate` |
| 监控 | `monitor` | `alert_monitor` | `go/services/monitor-svc/migrations/` | `monitor-svc -migrate` |
| 事件 | `episode` | `alert_episode` | `go/services/episode-svc/migrations/` | `episode-svc -migrate` |
| 任务 | `task` | `alert_task` | `go/services/task-orchestrator/migrations/` | `task-orchestrator -migrate` |
| 认知 | `cognitive` | `alert` | `python/agent-runtime/migrations/` | `python -m alert_agent.bootstrap.migrate` |

## 规则
- schema/role 由 `deploy/split/provision.sh` 创建（`--profile setup run --rm provision`），并设 `ALTER DEFAULT PRIVILEGES` 覆盖后续新表。
- 迁移 forward-only；每个域只读写自有 schema。隔离由 `deploy/split/verify-isolation.sh` 校验（期望 8/8）。
- 共享契约（事件、DTO）放 `go/contracts`，不放任何域内部。