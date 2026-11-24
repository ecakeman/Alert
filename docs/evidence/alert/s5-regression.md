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