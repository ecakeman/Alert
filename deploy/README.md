# deploy

本地真实依赖编排（docker-compose，最终态 9 容器）。PostgreSQL 16(pgvector) + Apache Kafka（KRaft 单节点）。

## 起停

```bash
cp .env.example .env                                     # 含 COMPOSE_PROFILES=core
docker compose -f deploy/compose/compose.yaml up -d      # core：8 常驻 + kafka-init 一次性
docker compose -f deploy/compose/compose.yaml ps         # 等待 healthy
docker compose -f deploy/compose/compose.yaml down       # 保留卷；-v 才清库
```

## 运行单元与端口（容器内）

| 服务 | 端口 | 说明 |
|---|---|---|
| console-bff | 8080 | 对外 L1 聚合入口 |
| service-registry | 8081 | 目录域 |
| episode-svc | 8082 | 事件域 |
| monitor-svc | 8083 | 监控域 |
| task-orchestrator | 8084 / 9090 | 任务域（HTTP/gRPC） |
| agent-runtime | 50052 | 认知侧（gRPC） |

默认 core **不暴露** 8081/8083 等宿主端口；排障叠加 `-f deploy/compose/compose.debug.yaml`。

## Kafka listener 与 topic

- **双 listener**：容器内用 `kafka:29092`（INTERNAL）；宿主进程用 `127.0.0.1:9092`（EXTERNAL），混用会拿到不可路由地址。
- **按域 topic**：`kafka-init` 幂等创建 4 个（3 分区/1 副本）：`alertflow.{registry,monitor,episode,task}.events.v1`。
- **旧通道已停用**：`alertflow.system.events.v1` 仅历史数据、消费者组为空、无新增生产；配置中零命中。
- `compose.p1~p5.yaml` 为历史快照，**不用于运行**。

手工重建 topic：`docker compose -f deploy/compose/compose.yaml run --rm kafka-init`

## 域账号隔离（setup profile）

```bash
docker compose -f deploy/compose/compose.yaml --profile setup run --rm provision
```

创建 4 schema / 4 role：`registry→alert_registry`、`monitor→alert_monitor`、`episode→alert_episode`、`task→alert_task`。
隔离：`REVOKE ALL ON SCHEMA public FROM PUBLIC`；各角色仅自有 schema 权限，跨 schema 在库层被拒。
反向验证：`bash deploy/split/verify-isolation.sh`（期望 `PASS 8/8`）。

## 迁移

```bash
for s in service-registry monitor-svc episode-svc task-orchestrator; do
  docker compose -f deploy/compose/compose.yaml run --rm "$s" -migrate
done
docker compose -f deploy/compose/compose.yaml run --rm agent-runtime python -m alert_agent.bootstrap.migrate
```

## 门槛

```bash
bash scripts/verify-p6.sh --only=direct|bridge|docs|all
bash scripts/verify-p7.sh --structure-only
bash deploy/split/verify-isolation.sh
```

## 已知限制

- 宿主 Go 进程用 `127.0.0.1:9092`，容器内用 `kafka:29092`，混用失败。
- 示例口令 `alert/alert` 与各域默认口令仅限本地，非真实凭据。