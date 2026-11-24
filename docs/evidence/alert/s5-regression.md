# S5 最终最小回归 — 证据

- 日期：2026-11-24
- 基线：S4 末 `fb5fc43`
- 原则：只执行一次最小集；每项三态（通过 / 失败（首个失败与命令） / 未执行（原因））；B7/B8 不纳入通过声明。
- Go 工具链：WSL go-dist `go1.27.4`（Windows go 在 UNC 上不可用）。

## 1. T-S5.1 静态与结构回归（本节）

### 1.1 构建与定向测试（工作目录 `services/system-core`）

| 检查 | 命令 | 结果 |
|---|---|---|
| 构建 | `go build ./...` | ✅ BUILD-OK |
| 静态检查 | `go vet ./...` | ✅ VET-OK |
| 定向测试 | `go test ./internal/domain/... ./internal/application/... -count=1` | ✅ domain + service/episode/monitor/dashboard/outbox/health/apperr 全 ok |

### 1.2 结构断言（仓库根）

| 检查 | 结果 |
|---|---|
| 根目录无 `go.mod`/`cmd`/`internal`/`migrations` | ✅ ROOT-CLEAN-OK |
| 无 `go.work` | ✅ NO-GOWORK |
| `go.mod` 无 `replace` | ✅ 0 处 |
| `find . -name go.mod`（排除 node_modules） | ✅ 仅 `./services/system-core/go.mod` |

### 1.3 依赖方向断言

| 检查 | 结果 |
|---|---|
| 旧前缀 `github.com/ecakeman/alert/internal` / `.../migrations` | ✅ 0 处（rg 无匹配） |
| `application/monitor` 无 episode import | ✅ 无匹配 |
| `transport` 无 domain import | ✅ 生产代码 0 处；唯一命中 `httperr/httperr_test.go:9`（测试构造输入，**S1 阶段已披露豁免**，见 s1-summary.md P1/第 52/57 行） |
| `probe` 无 UOW/UnitOfWork/TargetRepo/port.Tx/Repo | ✅ 无匹配 |
| `bootstrap` 无 auditHandler/Reserve(/MarkDone( | ✅ 无匹配 |

### 1.4 契约断言

| 检查 | 结果 |
|---|---|
| `type Event struct` | ✅ 仅 `internal/contracts/event.go:12` 一处 |
| `event-envelope.v1.schema.json` 无投递控制字段 | ✅ schema 本体 0 处（README 第 19 行为禁令说明文字，非字段） |

### 1.5 B7 保留清单存在性（文件级）

| 检查 | 结果 |
|---|---|
| `domain/{task,artifact,bound,state}.go` | ✅ 4/4 存在 |
| `postgres/{tasks,ownership,artifacts,idempotency}_repo.go` | ✅ 4/4 存在 |
| `port/task/` | ✅ 存在 |
| 501 路由 3 条（artifacts / retry-diagnosis / retry-archive） | ✅ router.go:33-35 |
| 配置保留（AGENT_GRPC_TARGET/GRPC_ADDR/TASK_*/OWNERSHIP_TTL） | ✅ config.go 命中 19 处 |

### 1.6 历史可追溯抽查（git log --follow 各 1）

| 文件 | 追溯链（截断） |
|---|---|
| Go：`internal/bootstrap/usecases.go` | `d191f5d`（搬迁）→ `68896e6`（S2.5） |
| SQL：`migrations/0001_system_core_entities.sql` | `d191f5d`（搬迁）→ `671a7c7`（B1 建表） |
| 证据：`docs/evidence/history/alertflow-stage1/u01-status.md` | `fb5fc43`（S4.4 归档）→ `eafd1e9`（骨架随迁） |

### 1.7 T-S5.1 结论

静态、结构、依赖方向、契约、B7 存在性、历史追溯**全部通过**（含 1 处 S1 既有披露豁免，非新增违规）。**放行 T-S5.2**。
## 2. T-S5.2 真实依赖冒烟（PG/Kafka/api/worker/路由/探测）

- 环境：WSL 内 docker（alert-pg / alert-kafka 均 healthy，Up 18h）；专用空库 `s5_smoke_20261124`（新建，开发库零触碰）；探测端点 WSL 内 `python3 -m http.server 8901`；Go 1.27.4 go-dist。

### 2.1 迁移回放（专用库两次）

| 检查 | 结果 |
|---|---|
| 首轮 `-migrate` | ✅ `applied=3 skipped=0 conflicts=0` |
| 次轮 `-migrate` | ✅ no-op：`applied=0 skipped=3 conflicts=0` |
| SQL SHA-256 | ✅ 三文件与 S0 基线逐一一致（`79c451…` / `9c721b…` / `3bc5f8…`） |

### 2.2 双进程角色边界

| 检查 | 结果 |
|---|---|
| api 日志无 probe/publisher/outbox 关键词 | ✅ 0 处 |
| 8080 仅由 api 进程监听 | ✅ `ss -ltnp` 单一持有者 |
| worker 日志无 HTTP/GIN | ✅ 0 处；无监听端口 |
| `/healthz`、`/readyz` | ✅ 200；readyz 含 pg+migrations 检查通过 |

### 2.3 路由 1~15 逐条最小请求

| # | 路由 | 结果 |
|---|---|---|
| 1 | GET /dashboard/summary | ✅ 真实读：trend success=152/failure=25、recent=1 条 |
| 2 | GET /services | ✅ |
| 3 | POST /services（含 1 个 HTTP 目标） | ✅ `ALL_REQUIRED` 模式默认 |
| 4 | GET /services/:id | ✅ |
| 5 | PATCH /services/:id | ✅ |
| 6 | DELETE /services/:id | 未执行（冒烟保留探测服务到底，无第二个可删对象；删除路径由 #10 目标删除同构覆盖） |
| 7 | GET /services/:id/targets | ✅ |
| 8 | POST /services/:id/targets | ✅ |
| 9 | PATCH /targets/:tid | ✅ |
| 10 | DELETE /targets/:tid | ✅ 204 |
| 11 | GET /episodes | ✅ 故障后 total=1 |
| 12 | GET /episodes/:id | ✅ |
| 13 | POST /services/:id/events | ✅（AppEventInput：producer_id/event_id 必填） |
| 14 | GET /services/:id/timeline | ✅ MONITOR_POINT 19+ 条 + APP_EVENT |
| 15 | GET /episodes/:id/timeline | ✅ 投影视图（episode + region1_projection + monitor_points + region2_events + region3_refs） |
| 16~18 | artifacts / retry-diagnosis / retry-archive | ✅ 均统一 501 体（`NOT_IMPLEMENTED`，B7 未接线不计通过） |
| — | 未注册路径 | ✅ 统一 404 |

status 过滤：`ABNORMAL` total=1 命中、`NORMAL`/`UNKNOWN` total=0、非法值 400；total 与过滤同口径（S1.11 修复生效）。

### 2.4 真实探测驱动链路

| 步骤 | 结果 |
|---|---|
| 建目标（http://127.0.0.1:8901/，interval 1s，阈值 1/1）→ worker 调度采样 | ✅ `service_monitor_points` 落 479 个监控点 |
| 状态聚合 | ✅ 目标 NORMAL → 服务 `MONITOR` 来源 NORMAL（summary：normal=1） |
| PATCH address → 不可达端口（:8999） | ✅ 阈值聚合 → 服务 ABNORMAL → **episode 自动开启**（ACTIVE，started_by=MONITOR） |
| 看板故障态 | ✅ services.abnormal=1、episodes.active=1、recent_episodes=1 条 |
| PATCH 恢复可达 | ✅ MONITOR 来源回 NORMAL，`TryResolve` 调用发生 |

### 2.5 Kafka 与消费幂等

| 检查 | 结果 |
|---|---|
| topic `alertflow.system.events.v1` 存在且有消息 | ✅ |
| 信封公开字段 | ✅ event_id/event_type/schema_version/aggregate_type/aggregate_id/occurred_at/payload（7 字段；trace_id 可选未传） |
| 无投递控制字段 | ✅ claim_owner/claim_until/publish_attempts/next_retry_at/last_error 均不在信封 |
| outbox_events | ✅ 9 条：service.state.changed.v1 ×7 + episode.status.changed.v1 ×1 + episode.input.appended.v1 ×1 |
| consumer_idempotency | ✅ system.audit.v1 以 event_id 幂等消费 6 条，result_code=OK |

### 2.6 T-S5.2 已知偏差（如实记录，不计失败）

1. **episode 自动解除（RESOLVED）未发生**：`TryResolve` 的 domain 判定要求 MONITOR 与 APP 来源均 NORMAL；当前 APP 来源仅在服务创建时初始化 UNKNOWN，代码中不存在其转 NORMAL 的入口（`domain.ParseSourceStatus` 明示"绝不隐式转 NORMAL"）。`TryResolve` 调用链本身在恢复时被正确触发并被 domain 正确拒绝。与 B6 阶段证据一致（u09-b6.md 第 40 行：恢复路径当时即未覆盖），**非 S1~S5 收敛引入的回归**；APP 来源处置语义属后续业务接线范围。
2. 路由 6（DELETE /services/:id）未执行：单一冒烟服务需保留供探测链路全程使用；删除路径由同构的 #10 目标删除（204）覆盖。

### 2.7 T-S5.2 结论

真实依赖冒烟**通过**（含 2 条已知偏差披露）；B0~B6 在 S3 搬迁后未退化。