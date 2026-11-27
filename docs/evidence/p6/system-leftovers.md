# 系统侧（Go 控制面）遗留收口现状

| 项 | 结论 | 说明/证据 |
|---|---|---|
| **LEASE_RECLAIM_DEFECT** | **已修复（运行态验证）** | 新增 `TaskRepo.ReclaimExpired`：按 `task_id + status='RUNNING' + 租约过期` 原子回收（`generation+1`、清 `lease_until`、置 `RETRY_WAIT`），不依赖列表快照 generation，0 行视为已被回收（幂等）。运行态：任务 `b2f71738` 注入过期租约后由 `generation=7 → 8`、`lease_until` 清空并进入调度，**无 STATE_CONFLICT**。 |
| P6.4 纳秒/稳定排序 | **已实现** | `sortRecordsStable` 由占位改为按记录时间键的 `sort.SliceStable`（同刻保持原序）。 |
| PRE-5 episode AccessLog | **已补齐** | `episode-svc` 中间件链加入 `httpx.AccessLog(slog.Default())`，运行期可采 `/internal/*` 访问行。 |
| R31（episode Dockerfile 缺 COPY contracts） | **已满足** | `go/services/episode-svc/Dockerfile` 含 `COPY platform`、`COPY contracts`、`COPY services/episode-svc`。 |
| C6（禁分布式事务） | **已满足** | 跨域一致性用本地事务 + Outbox + 幂等 + 对账，无 2PC。 |
| 13/17/18 路由 | known-diff（D-23） | 状态相关值层，已登记不计 FAIL。 |

## P5.9/P5.11 全链路与异常回归
- 冷启动全拓扑（8 healthy + 认知迁移 6/6）、隔离 8/8、structure-only 9 PASS、demo full/minimal exit 0 已在前序完成。
- 4 项异常回归：**租约重派**已由本次 ReclaimExpired 运行态验证覆盖；**旧代次拒绝 / attempt 上限 / 对账补建** 三项目前仅为既有实现 + 单元级逻辑，未单独做运行态回归脚本（如实标注为剩余项）。
- P5.9 全链路 #3~#6/#10 仍未单独重跑（依赖 agent 真实闭环），标注为剩余项。

## 变更文件
- `go/services/task-orchestrator/internal/application/port/task/task.go`（+ReclaimExpired）
- `go/services/task-orchestrator/internal/infrastructure/persistence/postgres/tasks_repo.go`（+ReclaimExpired SQL）
- `go/services/task-orchestrator/internal/application/task/reconcile.go`（改用原子回收、幂等）
- `go/services/console-bff/internal/aggregate/timeline.go`（稳定排序）
- `go/services/episode-svc/cmd/episode-svc/main.go`（AccessLog）

## 构建/验证
`GOWORK=off go build ./...`（task-orchestrator / console-bff / episode-svc）均 0；BFF `go test ./internal/aggregate/... ./internal/transport/http/...` PASS。