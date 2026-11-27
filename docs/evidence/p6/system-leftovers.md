# 系统侧（Go 控制面）遗留收口现状

| 项 | 结论 | 说明/证据 |
|---|---|---|
| **LEASE_RECLAIM_DEFECT** | **已修复（运行态验证）** | 新增 `TaskRepo.ReclaimExpired`：按 `task_id + status='RUNNING' + 租约过期` 原子回收，`generation+1`、清 `lease_until`；`attempt>=max_attempts` → `FAILED(LEASE_RECLAIM_EXHAUSTED)`，否则 `RETRY_WAIT`（`$2::timestamptz` 显式类型）；0 行幂等。运行态见下“异常回归 A/B”。 |
| P6.4 纳秒/稳定排序 | **已实现** | `sortRecordsStable` 改为按记录时间键 `sort.SliceStable`（同刻保序）。 |
| PRE-5 episode AccessLog | **已补齐** | `episode-svc` 中间件链加入 `httpx.AccessLog(slog.Default())`。 |
| R31 episode Dockerfile COPY contracts | **已满足** | Dockerfile 含 `COPY platform`、`COPY contracts`、`COPY services/episode-svc`。 |
| C6 禁分布式事务 | **已满足** | 本地事务 + Outbox + 幂等 + 对账，无 2PC。 |
| 13/17/18 路由 | known-diff（D-23） | 状态相关值层，已登记不计 FAIL。 |

## P5.11 异常回归（`scripts/p5-anomaly-regressions.sh`，真实 PG + task reconciler）
`bash scripts/p5-anomaly-regressions.sh` → **SUMMARY PASS=3 FAIL=0**：
- **A 租约重派**：过期 RUNNING → `generation=2 / RETRY_WAIT / lease_until NULL` ✅
- **B attempt 上限**：`attempt=max_attempts` 过期 → `generation=2 / FAILED / LEASE_RECLAIM_EXHAUSTED` ✅
- **C 对账补建**：陈旧 DISPATCHED（10 分钟未受理）→ reconciler 触碰（updated_at 刷新、离开旧态，转 RUNNING/RETRY_WAIT）✅
- **D 旧代次拒绝**：应用层 fence token 需 gRPC 构造，脚本未覆盖 → **剩余项（如实）**。

## P5.9 全链路 #3~#6/#10
仍未单独重跑；依赖 agent 真实闭环（认知侧）。冷启动全拓扑（8 healthy + 认知迁移 6/6）、隔离 8/8、structure-only 9 PASS、demo full/minimal exit 0 已在前序完成 → **全链路 E2E 为剩余项**。

## 变更文件
- `task-orchestrator/internal/application/port/task/task.go`（+ReclaimExpired 接口）
- `task-orchestrator/internal/infrastructure/persistence/postgres/tasks_repo.go`（ReclaimExpired 原子 SQL + max_attempts→FAILED + `$2::timestamptz`）
- `task-orchestrator/internal/application/task/reconcile.go`（改用原子回收、幂等）
- `console-bff/internal/aggregate/timeline.go`（稳定排序）
- `episode-svc/cmd/episode-svc/main.go`（AccessLog）
- `scripts/p5-anomaly-regressions.sh`（新增，P5.11 异常回归）

## 构建/验证
`GOWORK=off go build ./...`（task-orchestrator / console-bff / episode-svc）= 0；BFF 单测 PASS；`p5-anomaly-regressions.sh` PASS=3/0。