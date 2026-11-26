# services/system-core

唯一 Go module：`github.com/ecakeman/alert/services/system-core`。REST/探针 + 后台循环双进程，六层结构，真实 PG/Kafka 依赖。

## 常用命令（工作目录：`services/system-core`）

```bash
go build ./...
go vet ./...
go test ./internal/domain/... ./internal/config/ ./internal/transport/http/router/
go run ./cmd/api -migrate      # 迁移回放（需 DATABASE_URL、KAFKA_BROKERS）
go run ./cmd/api               # REST 进程：HTTP + 探针，不启后台循环
go run ./cmd/worker            # 后台进程：DueTargets/probe/outbox/consumer，不启 HTTP
```

依赖：PostgreSQL 16 与 Kafka 由 `deploy/compose/compose.yaml` 提供；`DATABASE_URL` 缺失时启动与迁移均非零退出（失败路径语义）。

## 迁移说明

- 目录 `migrations/` 仅含 `.sql`（`embed.go` 以 `//go:embed *.sql` 嵌入，README 不进入运行时发现）。
- forward-only runner：发现 → 校验（版本/名称/checksum）→ 回放；重复执行 no-op。
- 回放入口 `go run ./cmd/api -migrate`；排障见 `docs/runbooks/migrate.md`。

## 目录六层速览

| 层 | 目录 | 职责 |
|---|---|---|
| domain | `internal/domain/` | 纯规则 + MONITOR 聚合（无 IO/框架） |
| contracts | `internal/contracts/` | Go 侧事件信封类型（与 `contracts/events/` 同源） |
| application | `internal/application/`（`port/` 七能力子包 + 用例） | 用例编排与事务边界 |
| infrastructure | `internal/infrastructure/`（postgres/messaging/probe） | 仓储/SQL/行模型、Kafka 收发、真实探测 |
| transport | `internal/transport/http/`（router/handler/dto） | 路由唯一注册处、薄适配 |
| bootstrap/config | `internal/bootstrap/`、`internal/config/` | api/worker 角色装配、表驱动配置（33 项） |

入口：`cmd/api/main.go`（REST + `-migrate`）、`cmd/worker/main.go`（后台）。