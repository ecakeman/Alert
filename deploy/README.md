# deploy

本地真实依赖编排（B0）：PostgreSQL 16 + Apache Kafka（KRaft 单节点）。

## 起停

```bash
docker compose -f deploy/compose/compose.yaml up -d
docker compose -f deploy/compose/compose.yaml ps     # 等待 postgres / kafka 均 healthy
docker compose -f deploy/compose/compose.yaml down   # 保留数据卷；加 -v 才会清库
```

## 建库（B1 / B2 前置）

```bash
docker exec alert-pg psql -U alert -c 'CREATE DATABASE alert_test'
docker exec alert-pg psql -U alert -c 'CREATE DATABASE alert_migrate_check'
```

## 已知限制

- Kafka 仅单 listener（`PLAINTEXT://127.0.0.1:9092`），对宿主机上的 Go 进程有效；容器间访问不适用（当前应用尚未容器化，属已知限制而非降级）。
- 示例口令 `alert/alert` 仅限本地使用，非真实凭据。