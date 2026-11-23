# U06 B2 persistence 端口真实实现 — 证据

- 日期：2026-11-23
- 工程根：`/home/sancho/workspace/projects/Alert`
- 真实依赖：`postgres:16` healthy；Go 1.27.4；测试库 `alert_test`（14 表）
- 原则：零降级；真实 pgx SQL，无内存仓储分支。

## 1. 目标与范围

在 `application/port` 定义 18 个端口，在 `infrastructure/persistence/postgres` 用真实 PostgreSQL 实现 13 个仓储；错误按约束名翻译；UoW 一次用例一事务。

**本块不做**：application 业务编排（B3）、Handler（B4）、Kafka（B5）、探测（B6）、gRPC（B7）。

## 2. 端口与仓储

| 端口 | 方法 | 实现 |
|---|---|---|
| UnitOfWork | `WithinTx(ctx, fn(Tx) error)` | `postgres/uow.go`（READ COMMITTED；40001/40P01 一次重试） |
| Clock / IDGen | `Now` / `NewID` | 接口（B3 注入） |
| ServiceRepo | Create/Get/GetByKey/List/Patch/SoftDelete | `services_repo.go` |
| TargetRepo | Create/Get/ListByService/Patch/SoftDelete | `targets_repo.go` |
| SourceStateRepo | Get/ListByService/Upsert | `source_states_repo.go` |
| EpisodeRepo | GetActiveByService/Get/List/Create/UpdateStatus/TouchInputVersion | `episodes_repo.go` |
| InputVersionRepo | AppendIfChanged/GetLatest/ListByEpisode | `input_versions_repo.go` |
| MonitorPointRepo | Append/ListByBound/LatestSeqByService | `monitor_points_repo.go` |
| AppEventRepo | GetByIdentity/Append/ListByBound | `app_events_repo.go` |
| TaskRepo | Create/Get/ListActiveByEpisode/UpdateCAS/ListDueRetry/ListDispatchPending | `tasks_repo.go` |
| OwnershipRepo | Acquire/Get/Release/ReclaimExpired | `ownership_repo.go` |
| OutboxRepo | Append/Claim/MarkPublished/MarkFailed | `outbox_repo.go` |
| ArtifactRepo | GetLatest/ListByEpisode/Insert | `artifacts_repo.go` |
| ConsumerIdempotencyRepo | Reserve/MarkDone | `idempotency_repo.go` |
| DashboardRepo | Summary/Counts/Trend | `dashboard_repo.go` |

## 3. 命令矩阵与三态

| 检查 | 命令 | 结果 |
|---|---|---|
| build | `go build ./...` | 通过 |
| vet | `go vet ./...` | 通过 |
| application 依赖方向 | `go list -deps ./internal/application/...` 查 gin/http/pgx/kafka | 通过（CLEAN） |
| 无 SQL 外泄 | `grep` application 目录 SELECT/INSERT/UPDATE/DELETE | 通过（NONE） |
| alert_test 迁移 | `-migrate` | 通过（applied=3，14 表） |
| 集成测试 | `TEST_DATABASE_URL=... go test ./.../postgres/...` | 通过（7/7 PASS） |

## 4. 集成用例（真实 PG）

| 用例 | 断言 | 结果 |
|---|---|---|
| TestRepoDuplicateServiceKey | 同 service_key 第二次 → `ALREADY_EXISTS` | PASS |
| TestRepoNotFound | 不存在 id → `NOT_FOUND` | PASS |
| TestRepoOneActiveEpisode | 同服务两条 ACTIVE → `ALREADY_EXISTS`（`episodes_one_active`） | PASS |
| TestRepoOutboxClaimExclusive | 第一次 Claim 取到、第二次 Claim 为 0（claim 排他） | PASS |
| TestRepoTransactionRollback | 事务内写 service+outbox 后失败 → 双表回滚，service 查不到 | PASS |
| TestNewPoolInvalidDSN | 非法 DSN 错误不泄露 | PASS |
| TestNewPoolReal | 真实连接成功 | PASS |

## 5. 关键实现

- **并发保护在 SQL**：`GetActiveByService` 用 `FOR UPDATE`；`Outbox.Claim` 用 `FOR UPDATE SKIP LOCKED`；`Task.UpdateCAS` 以 `generation + 允许状态 + lease_until > now()` 为条件。
- **错误翻译**：`errors.go` 按 B1 固定约束名 → `ALREADY_EXISTS`；`23514` → `VALIDATION`；`23503` → `STATE_CONFLICT`；无行 → `NOT_FOUND`；不泄露 SQL/DSN。
- **UTC**：`time.go` 统一 `.UTC()`，所有 `timestamptz` 读写。
- **无内存分支**：生产代码无 `memory/inmem` 仓储；测试库为真实 PG。

## 6. 未执行项

- 全量/E2E 未执行（展示级最小集）。
- 并发压测未执行；并发语义由 SQL 行锁/部分唯一/CAS 表达，并以单 ACTIVE、claim 排他最小用例覆盖。

## 7. 下一块入口

**AB-B3**：application 真实用例（service/episode/dashboard），依赖 B2 端口；事务边界与 Outbox 同事务写入。

## 8. 阻塞项

- 无。真实 PG 可用，13 仓储编译与 5 类集成语义均已验证。