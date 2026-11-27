# 故障排查

## 域服务未随 `up -d` 启动
- 现象：只有 pg/kafka/kafka-init 起来。
- 原因：未启用 core profile。
- 处理：`.env` 设 `COMPOSE_PROFILES=core`，或 `docker compose -f deploy/compose/compose.yaml --profile core up -d`。

## monitor-svc `/readyz` 非 200（catalog）
- 空库/空目录：投影为空即视为已同步（`catalogSyncedBefore` 空投影分支），应 200。
- 有存量服务：需等待 registry catalog 事件被消费（`monitor.consumer_idempotency` 记录出现）后转 healthy。

## Kafka 连不上
- 容器内代码用 `kafka:29092`（INTERNAL）；宿主机进程用 `127.0.0.1:9092`（EXTERNAL）。混用会失败。

## 根目录 `go build ./...` 报错
- 正常：根不是 module。逐 module `GOWORK=off go build ./...`（见 build.md）。

## 端口冲突
- 默认 core 无 8081/8083 宿主映射；需要排障时叠加 `-f deploy/compose/compose.debug.yaml`，用完撤下。

## 迁移冲突
- 输出 `conflicts>0` 表示历史迁移校验和不一致；forward-only，需人工核对 `migrations/` 与 `schema_migrations`，禁止手改已应用文件。