# Alert（微服务版）

告警事件处理平台。Go 多 module 域服务 + console-bff 聚合 + Python agent-runtime（认知侧），
按 Strangler 从单体演进为 4 域微服务，最终 9 容器拓扑。

## 最终拓扑（9 容器 = 8 常驻 + kafka-init 一次性）

| 服务 | 职责 | 端口(容器) |
|---|---|---|
| console-bff | L1 18 路由反代 + 聚合（dashboard/列表/详情/时间轴） | 8080 |
| service-registry | 目录域（服务/目标/来源状态） | 8081 |
| monitor-svc | 监控域（探测/时间点/趋势） | 8083 |
| episode-svc | 事件域（episode/输入版本/时间轴分片） | 8082 |
| task-orchestrator | 任务域（诊断/归档/产物引用，gRPC 9090） | 8084 |
| agent-runtime | 认知侧（诊断 agent + RAG，gRPC 50052） | 50052 |
| postgres / kafka | 基础设施（pgvector / KRaft） | 5432 / 9092 |

## 快速开始（5 步）

```bash
cp .env.example .env                                  # 含 COMPOSE_PROFILES=core
docker compose -f deploy/compose/compose.yaml up -d   # core profile：8 常驻 + kafka-init
docker compose -f deploy/compose/compose.yaml --profile setup run --rm provision   # 4 schema/4 role
for s in service-registry monitor-svc episode-svc task-orchestrator; do
  docker compose -f deploy/compose/compose.yaml run --rm "$s" -migrate
done
docker compose -f deploy/compose/compose.yaml run --rm agent-runtime python -m alert_agent.bootstrap.migrate
# 访问 http://localhost:8080 ；一键演示 scripts/demo.sh
```

> 未设置 `COMPOSE_PROFILES=core` 时 `up -d` 不会启动域服务；`--profile debug` + `compose.debug.yaml`
> 才暴露宿主端口 8081/8082/8083/8084/9090（仅排障）。

## 构建与验证

- 构建（多 module，禁用根 `go build ./...`）：逐 module `GOWORK=off go build ./...`。
- 门禁：`bash scripts/verify-p6.sh --only=direct|bridge|docs`；`bash scripts/verify-p7.sh --structure-only`；`bash deploy/split/verify-isolation.sh`。
- 一键演示：`bash scripts/demo.sh`（或 `--minimal`）。

## 文档

- 运行手册：`docs/runbooks/{run-local,smoke,troubleshoot,build,migrate}.md`
- 架构：`docs/architecture/{overview,data-ownership,contracts}.md`
- 部署与 topic：`deploy/README.md`；证据：`docs/evidence/p6/README.md`（p6.1~p6.8）。