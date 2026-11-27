# 架构总览

按 Strangler 从单体演进为 4 个 Go 域服务 + console-bff 聚合 + Python 认知侧，最终 **9 容器**。

## 代码结构（双语言分区）

```
go/                       # Go 大块（构建根，go.work 引用各 module）
├─ contracts/             # 共享契约 module（事件信封、域间 DTO、trace 键）
├─ platform/              # 共享技术能力（pool/迁移/outbox/kafka/httpx/apierr）
└─ services/              # 每个可独立部署单元一个 module
   ├─ console-bff/        # L1 18 路由反代 + 聚合
   ├─ service-registry/   # 目录域
   ├─ monitor-svc/        # 监控域
   ├─ episode-svc/        # 事件域
   └─ task-orchestrator/  # 任务域
python/agent-runtime/     # 认知侧（诊断 agent + RAG），gRPC 50052
deploy/compose/           # 编排（compose.yaml / compose.p1~p5 历史快照 / compose.debug.yaml）
scripts/                  # 门禁与演示
docs/                     # 架构、运行手册、证据
```

## 运行单元

| 单元 | 角色 | 端口 |
|---|---|---|
| console-bff | 对外聚合入口（L1） | 8080 |
| service-registry | 服务/目标/来源状态 | 8081 |
| monitor-svc | 探测、时间点、趋势 | 8083 |
| episode-svc | episode、输入版本、时间轴分片 | 8082 |
| task-orchestrator | 诊断/归档任务、产物引用（gRPC） | 8084 / 9090 |
| agent-runtime | 认知侧（诊断/RAG） | 50052 |
| postgres / kafka | 基础设施 | 5432 / 9092(29092) |

## 关键链路
- 同步：BFF L1 路由 → 域服务 L2 `/internal/*`（registry/monitor/episode/task），BFF 负责聚合与契约投影。
- 异步：各域 outbox → Kafka 4 按域 topic → 下游投影消费（含幂等表去重）。
- 任务：episode 事件 → task-orchestrator → gRPC 调 agent-runtime → 产物写入并回写引用。