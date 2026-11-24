# U06 B0 真实运行基座 — 证据

- 日期：2026-11-23
- 工程根：`/home/sancho/workspace/projects/Alert`
- 分支/形态：本地开发（Go 1.27.4 + 真实 PostgreSQL16 + 真实 Kafka KRaft）
- 原则：零降级、分块真实实现；本块只做运行基座，不做业务表/业务 Handler/Kafka 客户端/探测/gRPC。

## 1. 目标与范围

B0 目标：建立一条命令可起的真实依赖（PG16 + Kafka），配备连接池、配置两段校验、forward-only 迁移 runner、错误六档与统一 404、真实启动装配与退出码、真实就绪探针。

**本块不做**：业务表（B1）、业务 Handler（B4/B7）、Kafka 客户端（B5）、HTTP/TCP 探测（B6）、gRPC/Python（B7/B8）。

## 2. 真实依赖版本

| 组件 | 版本/来源 |
|---|---|
| PostgreSQL | 镜像 `postgres:16`，容器 `alert-pg`，端口 `5432` |
| Kafka | 镜像 `apache/kafka:3.8.0`（KRaft 单节点），容器 `alert-kafka`，端口 `9092/9093` |
| pgx | `github.com/jackc/pgx/v5 v5.13.0`（含 `puddle/v2 v2.2.3`、`x/sync v0.22.0`） |
| Go | `go1.27.4 linux/amd64`（`~/.local/go`） |
| 本地口令 | `alert/alert`（仅本地示例，非真实凭据） |

## 3. 改动清单（逐文件）

- `deploy/compose/compose.yaml`（新增：PG16 + Kafka KRaft）
- `deploy/README.md`（起停/建库/已知限制）
- `.env.example`（扩到 33 项）
- `internal/config/config.go`、`config_test.go`（7→33 项，Load/ValidateRuntime 两段）
- `go.mod`、`go.sum`（新增 pgx/v5 及传递依赖）
- `internal/infrastructure/persistence/postgres/pool.go`、`doc.go`、`pool_test.go`（唯一连接池装配）
- `internal/infrastructure/persistence/postgres/checks.go`（`pg` / `migrations` 双 checker）
- `migrations/embed.go`、`migrations/README.md`（`//go:embed *` 于 migrations 包）
- `internal/infrastructure/persistence/migrate/runner.go`、`runner_test.go`、`integration_test.go`（纯逻辑 + 库侧）
- `internal/application/apperr/errors.go`（应用层稳定码）
- `internal/transport/http/httperr/httperr.go`、`httperr_test.go`（六档映射）
- `internal/transport/http/router/router.go`、`router_test.go`（route 结构加 handler、NoRoute 404）
- `internal/bootstrap/app.go`（New 返回 error、装配、-migrate 支撑、双 checker）
- `cmd/system-core/main.go`（`-migrate` 入口、两段配置、退出码）
- `docs/evidence/u06-business-b0.md`（本文件）

## 4. 命令矩阵与三态

| 检查 | 命令 | 结果 | 首因 |
|---|---|---|---|
| A1 build | `go build ./...` | 通过 | — |
| A2 vet | `go vet ./...` | 通过 | — |
| A3 domain 14 用例 | `go test ./internal/domain/... -count=1` | 通过（14 用例） | — |
| 块级测试 | `go test ./internal/... -count=1` | 通过（config/domain/migrate/postgres/httperr/router） | — |
| A6 依赖方向 | `go list -deps ./internal/application/...` 查 gin/http/pgx/kafka | 通过（CLEAN） | — |
| A6 依赖方向 | `go list -deps ./internal/transport/http/...` 查 infrastructure | 通过（CLEAN） | — |
| compose 语法 | `docker compose ... config -q` | 通过 | — |
| 依赖健康 | `docker compose ... ps` | 通过（postgres/kafka 均 healthy） | — |
| PG 连接 | `docker exec alert-pg pg_isready -U alert -d alert` | 通过（accepting connections） | — |
| Kafka | `kafka-topics.sh --bootstrap-server localhost:9092 --list` | 通过（exit 0，空列表） | — |
| A7 迁移入口 | `-migrate`（DATABASE_URL=alert） | 通过（applied=0 skipped=0 conflicts=0，exit 0） | — |
| 迁移集成 | `TEST_DATABASE_URL=... go test ./.../migrate/...` | 通过（TestUpIdempotent 真实 PG） | — |
| 版本表 | `information_schema.columns` | 通过（5 列结构正确） | — |
| A5 healthz | `curl /healthz` | 通过（200 `{"status":"ok","checks":[]}`） | — |
| 就绪 | `curl /readyz` | 通过（200，pg+migrations ok） | — |
| 断库 | 停 `alert-pg` 后 `curl /readyz` | 通过（503 DEPENDENCY_UNAVAILABLE） | — |
| 恢复 | 启 `alert-pg` 后 `curl /readyz` | 通过（200） | — |
| 404 | `curl /api/v1/not-a-route` | 通过（404 NOT_FOUND + request_id） | — |
| 501 | `curl /api/v1/services` | 通过（501 NOT_IMPLEMENTED + request_id） | — |
| 缺配置 | `env DATABASE_URL= ...` | 通过（exit 1，仅报变量名） | — |
| 422 残留 | 检索 `internal/transport` | 通过（无 422） | — |

## 5. 路由状态表（18 条，B0 阶段全部 501）

| # | METHOD | PATH | 状态 |
|---|---|---|---|
| 1 | GET | /api/v1/dashboard/summary | 501（未施工） |
| 2 | GET | /api/v1/services | 501（未施工） |
| 3 | POST | /api/v1/services | 501（未施工） |
| 4 | GET | /api/v1/services/:service_id | 501（未施工） |
| 5 | PATCH | /api/v1/services/:service_id | 501（未施工） |
| 6 | DELETE | /api/v1/services/:service_id | 501（未施工） |
| 7 | GET | /api/v1/services/:service_id/targets | 501（未施工） |
| 8 | POST | /api/v1/services/:service_id/targets | 501（未施工） |
| 9 | PATCH | /api/v1/targets/:target_id | 501（未施工） |
| 10 | DELETE | /api/v1/targets/:target_id | 501（未施工） |
| 11 | GET | /api/v1/episodes | 501（未施工） |
| 12 | GET | /api/v1/services/:service_id/timeline | 501（未施工） |
| 13 | POST | /api/v1/services/:service_id/events | 501（未施工） |
| 14 | GET | /api/v1/episodes/:episode_id | 501（未施工） |
| 15 | GET | /api/v1/episodes/:episode_id/timeline | 501（未施工） |
| 16 | GET | /api/v1/episodes/:episode_id/artifacts | 501（未施工） |
| 17 | POST | /api/v1/episodes/:episode_id/retry-diagnosis | 501（未施工） |
| 18 | POST | /api/v1/episodes/:episode_id/retry-archive | 501（未施工） |

## 6. 未执行项与原因

- 全量/E2E/`-race`：按展示级最小集原则未执行。
- 历史 `integration-e2e.md` 结论：不适用于新工程，未复用。
- B0 无业务迁移：`expected=0`（业务表在 B1 才建）。

## 7. 已知偏差

1. **口令口径**：spec 示例 `postgres:alert`，design 用 `alert:alert`；统一取 `alert/alert`（本地示例）。
2. **Kafka listener**：仅单 listener `PLAINTEXT://127.0.0.1:9092`，对宿主机 Go 进程有效；容器间访问不适用（应用未容器化，属已知限制而非降级）。
3. **embed 位置**：Go 禁止 `//go:embed ..`，改用 `migrations` 包目录 `//go:embed *`，runner 过滤 `*.sql`。
4. **启动依赖**：B0 起启动真实要求 `DATABASE_URL`（失败退出码 1），覆盖骨架期“零依赖启动”；这是“零降级”的业务要求，`/healthz` 仍不依赖外部。
5. **gin 依赖**：`go mod tidy` 后 gin 为直接依赖；pgx 新增后 `x/text` 升级到 v0.41.0。
6. **块级测试范围**：仅本块新增包 + 既有 domain/config/transport，不跑全仓业务测试。

## 8. 下一块入口

**AB-B1.1**：迁移 `0001_system_core_entities.sql`（services / service_monitor_targets / service_source_states / episodes 四表），含命名约束 `episodes_one_active` 等。前置 B0.5 已满足。

## 9. 阻塞项

- 无（PG/Kafka 均 healthy；迁移入口与探针已实测通过）。
- 提醒：WSL 内 Go 需 `export PATH=$HOME/.local/go/bin:$PATH`；未写入 shell 配置。