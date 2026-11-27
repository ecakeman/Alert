# 本地运行

最终态为 **9 容器**（8 常驻 healthy + `kafka-init` 一次性 exited 0）。

## 启动

```bash
cp .env.example .env                                   # 含 COMPOSE_PROFILES=core
docker compose -f deploy/compose/compose.yaml up -d    # core profile：基础设施 + 6 域服务
docker compose -f deploy/compose/compose.yaml --profile setup run --rm provision
for s in service-registry monitor-svc episode-svc task-orchestrator; do
  docker compose -f deploy/compose/compose.yaml run --rm "$s" -migrate
done
docker compose -f deploy/compose/compose.yaml run --rm agent-runtime python -m alert_agent.bootstrap.migrate
```

- 访问控制台聚合：`http://localhost:8080`（`/readyz` 应为 200）。
- 一键演示：`bash scripts/demo.sh`（或 `--minimal`）。

## 服务与端口（容器内）

| 服务 | 端口 |
|---|---|
| console-bff | 8080 |
| service-registry | 8081 |
| episode-svc | 8082 |
| monitor-svc | 8083 |
| task-orchestrator | 8084（gRPC 9090） |
| agent-runtime | 50052 |
| postgres / kafka | 5432 / 9092（容器内 29092） |

> 默认 core 不暴露 8081/8083 等宿主端口；排障用 `-f compose.debug.yaml` 叠加（暴露 8081/8082/8083/8084/9090）。