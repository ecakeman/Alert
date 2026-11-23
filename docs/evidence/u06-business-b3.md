# U06 B3 application 真实用例 — 证据

- 日期：2026-11-23
- 工程根：`/home/sancho/workspace/projects/Alert`
- 真实依赖：`postgres:16` healthy；`alert_test`（14 表）；Go 1.27.4
- 原则：零降级；用例只调用 domain 判定，写用例一事务，业务事实 + Outbox 同事务。

## 1. 目标与范围

实现 `application/{service,episode,dashboard}` 真实用例，把 domain 规则接入业务编排；事务边界与 Outbox 同事务写入。

**本块不做**：HTTP Handler（B4）、Kafka 发送（B5）、探测（B6）、gRPC（B7）。

## 2. 用例清单

| 包 | 用例 | 关键点 |
|---|---|---|
| `application/service` | Create / List / Get / Patch / SoftDelete / ListTargets / CreateTarget / PatchTarget / DeleteTarget | 每写用例一 `WithinTx`；创建含 2 来源状态 + 初始目标 + Outbox；目标增删改后重算 MONITOR 来源 |
| `application/episode` | OpenOrReuse / List / Get / AppendAppEvent / AppendInputVersion / TryResolve | `DecideIncidentOnAbnormal`、`DecideAppEvent`、`Fingerprint`、`CanResolve`；输入/事件/解除同事务写 Outbox |
| `application/dashboard` | Summary | 只读聚合，复用 `DashboardRepo` |

## 3. 命令矩阵与三态

| 检查 | 命令 | 结果 |
|---|---|---|
| build | `go build ./...` | 通过 |
| vet | `go vet ./...` | 通过 |
| domain 14 用例 | `go test ./internal/domain/...` | 通过 |
| application 依赖方向 | `go list -deps ./internal/application/...` 查 gin/http/pgx/kafka | 通过（CLEAN） |
| 应用层集成 | `TEST_DATABASE_URL=... go test ./internal/application/...` | 通过（全 PASS） |

## 4. 集成用例（真实 PG）

| 用例 | 断言 | 结果 |
|---|---|---|
| TestCreateServiceAndDuplicate | 创建成功；重复 service_key → `ALREADY_EXISTS` | PASS |
| TestGetNotFound | 不存在 id → `NOT_FOUND` | PASS |
| TestOpenOrReuse | 首次创建、二次复用同 episode | PASS |
| TestAppendAppEventIdempotentAndConflict | 同键同内容幂等；不同内容 → `EVENT_ID_CONFLICT` | PASS |
| TestAppendInputVersionIdempotent | 指纹相同不新增、首轮新增 | PASS |
| TestTryResolveBlocked | 条件不满足时不解除并返回缺失项 | PASS |
| TestSummary | 看板聚合返回非空 maps | PASS |

## 5. 关键实现与范围调整

- **domain 规则为唯一判定**：`OpenOrReuse` 用 `DecideIncidentOnAbnormal`；`AppendAppEvent` 用 `DecideAppEvent`；`AppendInputVersion` 用 `Fingerprint`；`TryResolve` 用 `CanResolve`。Handler 未实现任何状态机。
- **Outbox 同事务**：服务状态变更、事件追加、输入版本追加均在业务事务内 `Outbox.Append`（B5 才发送）。
- **app_event_seq**：`app_events_repo.Append` 增加 `SELECT ... FROM services ... FOR UPDATE` 行锁后按 `max(seq)+1` 分配（同服务事件串行）。属 B2→B3 的范围调整，已记录。
- **内容判定**：`domain.AppEvent` 增加 `ContentHash`，`SameAppEvent` 优先比较 content_hash（避免 jsonb 规范化导致误判冲突）；domain 14 用例仍通过。
- **来源重算**：目标增删改后按必需目标状态重算 MONITOR 来源；APP 来源不受目标影响。

## 6. 未执行项

- 全量/E2E 未执行（展示级最小集）。
- 并发压测未执行；关键并发语义由 B2 的 SQL 锁/唯一约束与最小用例覆盖。

## 7. 下一块入口

**AB-B4**：transport 路由 1~15 真实化（DTO + Handler + 查询参数钳制 + 前端 build），路由 16~18 保持 501。

## 8. 阻塞项

- 无。真实 PG 可用，application 用例与 domain 规则均已验证。