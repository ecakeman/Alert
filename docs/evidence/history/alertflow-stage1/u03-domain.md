> Source: migrated from AlertFlow stage-1 evidence (baseline e6aeb3d); conclusions unchanged.
# U03 业务不变量固定（纯规则）

- 记录日期：2026-11-23
- 代码基线：`e6aeb3d`（分支 `rework/e6aeb3d-baseline`）
- 规则包：`services/system-core/internal/domain`
- 依赖约束：无 `pgx` / `net/http` / `gin` / `grpc` / `internal/system`（`go list -deps` 检查 CLEAN）

## 1. 基线锚点表（T15）

| 职责（design 引用） | 基线实际位置 | 语义 | 影响单元 |
|---|---|---|---|
| 监控/APP 变更、Incident 状态、输入版本、投影 | `internal/system/apply.go` | 存在（职责混杂，待拆） | U03/U06 |
| Service CRUD 与事务封装 | `internal/system/store.go` | 存在（超级 Store） | U05 |
| Task 状态 / Lease / generation / 回调 / 重试 | `internal/system/task.go` | 存在（约 977 行，职责集中） | U07 |
| 查询 / 时间轴 / Agent 输入 | `internal/system/query.go` | 存在 | U09/U16 |
| Outbox Publisher | `internal/system/publish.go` | 存在（事务内做 Kafka I/O，待改） | U08 |
| 输入指纹白名单 | `internal/system/hash.go` | 存在（MonitorField / AppCanon） | U03 |
| 业务错误码 | `internal/system/error.go` | 存在（Code+HTTP，23505→409） | U09 |
| Schema 与迁移 | `internal/db/migrations/00001_init.sql` | 存在（同建 system+cognitive，待拆） | U21 |
| 监控引擎 / 调度 / 队列 / worker | `internal/monitoring/*` | 存在（scheduler 堆、bounded queue、worker） | U11 |
| 公开 API | `internal/api/server.go` | 存在（标准库 Handler，未直接对外监听） | U09 |
| gRPC / Kafka 入口 | `internal/gateway/{server,kafka}.go` | 存在 | U08/U17 |
| Python Runtime | `services/agent-runtime/src/alertflow_agent/runtime.py` | 存在（职责集中） | U12/U13 |
| Python Workflow | `services/agent-runtime/src/alertflow_agent/workflow.py` | 存在（dict State，单主 Agent） | U14 |
| Python receipt / artifacts / mcp / invocation | `services/agent-runtime/src/alertflow_agent/` | 存在（MCP 手写、A2A 经 Java） | U15/U18/U20 |

> 本表覆盖 design 关键职责；其余字段级/行号级锚点在进入对应单元时按同结构补齐。

## 2. INV 覆盖表（规则 + 最小用例）

| 不变量 | 规则落点 | 直接测试 | 结果 |
|---|---|---|---|
| INV-01 单活动故障 | `incident.go DecideIncidentOnAbnormal` | `TestINV01_SingleActiveIncident` | PASS |
| INV-02 双来源 + UNKNOWN | `source.go` | `TestINV02_SourceStatus` | PASS |
| INV-03 解除条件 | `incident.go CanResolve` | `TestINV03_CanResolve` | PASS |
| INV-04 指纹/版本 | `input.go Fingerprint` | `TestINV04_FingerprintStable` | PASS |
| INV-05 APP 追加/幂等/冲突 | `input.go DecideAppEvent` | `TestINV05_AppEventDecision` | PASS |
| INV-06 固定输入边界 | `bound.go` | `TestINV06_InputBound` | PASS |
| INV-07 服务级串行/跨服务并发 | `task.go CanAcquire` | `TestINV07_ServiceSerialization` | PASS |
| INV-08 generation/Lease fencing | `task.go Fence` | `TestINV08_Fence` | PASS |
| INV-09 结果不等于故障状态 | `task.go ApplyTaskResult`（结构不含 Incident） | `TestINV09_ResultIsolation` | PASS |
| INV-10 先解除后归档 | `state.go` + `artifact.go CanArchive` | `TestINV10_IncidentTransitions` / `TestINV10_CanArchive` | PASS |
| INV-11 双产物可追溯 | `artifact.go ValidateArtifactRef` | `TestINV11_ArtifactRef` | PASS |
| INV-12 终态不变/人工重试 | `state.go` + `task.go` | `TestINV12_TaskTerminalTransitions` / `TestINV12_TerminalAndRetry` | PASS |

## 3. 由后续单元集成覆盖的项

- INV-01 并发唯一性 → DB 唯一索引 `uniq_active_episode_per_service`（U06）。
- INV-07 真实跨 Incident 串行 / 跨 Service 并发 → U06 / T205。
- INV-11 契约字段一致性（Go↔Python）→ U15 / U16。

## 4. 未执行项

- DB 集成用例（T31 测试基座、T32 三用例）：按项目"测试只做必要最小集、展示级"总原则未执行，不影响纯规则层结论。