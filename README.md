# Alert

面向服务故障的监控、调查与可追溯诊断系统。当前形态：Go 模块化单体（`services/system-core`，Gin 对外 API + worker 双进程）+ Vue 控制台（`apps/console`），Python Agent Runtime 为占位组件。

## 仓库导航

| 目录 | 内容 |
|---|---|
| `services/system-core/` | 唯一 Go module（api + worker 双入口、六层结构、迁移） |
| `services/agent-runtime/` | Python 组件占位（B8 未实现，仅边界声明） |
| `apps/console/` | Vue 控制台（前端移植完成） |
| `contracts/` | 语言无关契约源（events 信封 Schema / proto 预留 / openapi 说明） |
| `docs/` | `architecture/`（结构与所有权）、`runbooks/`（操作手册）、`evidence/`（证据归档） |
| `deploy/compose/` | 本地 PG16 + Kafka(KRaft) 拓扑 |

## 当前能力矩阵

| 能力块 | 状态 |
|---|---|
| B0 运行基座（连接池 / 配置校验 / 迁移 runner / 统一 404 / 探针） | 完成 |
| B1 数据层（system schema 14 表 + 真实迁移回放） | 完成 |
| B2 persistence 仓储（真实 pgx，约束翻译，UoW） | 完成 |
| B3 application 用例（事务边界 + Outbox 同事务） | 完成 |
| B4 HTTP 路由 1~15（真实实现） | 完成 |
| B5 Outbox → Kafka 发布 + 幂等消费 | 完成 |
| B6 探测引擎（HTTP/TCP 真实探测 → 阈值聚合 → episode 驱动） | 完成 |
| B7 Agent 任务链（路由 16~18 / gRPC / task 端口） | 保留待接线（代码随迁保留，保持 501） |
| B8 Python Agent Runtime | 未实现（`services/agent-runtime/` 仅占位） |
| S1~S3 结构收敛（语义收敛 / 六层拆分 / 多组件目录搬迁） | 完成 |

## 启动（api + worker 双进程）

```bash
docker compose -f deploy/compose/compose.yaml up -d   # PG16 + Kafka
cd services/system-core
go run ./cmd/api -migrate                             # 首次迁移
go run ./cmd/api                                      # REST 进程（不启后台循环）
go run ./cmd/worker                                   # 后台进程（探测/outbox/消费，不启 HTTP）
```

配置见 `.env.example`（33 项，均有默认值，`DATABASE_URL` 需按本地环境修改）。详细操作见 `docs/runbooks/`。

## 最小验收

```bash
cd services/system-core && go build ./... && go vet ./...
```

组件级命令与冒烟路径：见 `services/system-core/README.md` 与 `docs/runbooks/smoke.md`。

## 已知限制

1. 路由 16~18（artifacts / retry-diagnosis / retry-archive）保留 501，属 B7 待接线。
2. 无 K8s 编排、无微服务拆分：单 Go module、api/worker 双进程入口。
3. 仅用于本地 / 可信内网演示，未接入鉴权；不代表生产可用。