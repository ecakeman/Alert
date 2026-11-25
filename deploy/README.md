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

## Kafka listener 与 topic

- **双 listener**：容器内代码使用 `kafka:29092`（`INTERNAL`）；宿主机上的 Go 进程使用 `127.0.0.1:9092`（`EXTERNAL`）。
- **按域 topic**：`kafka-init` 一次性容器创建 4 个 topic（3 分区 / 1 副本），幂等，可重复执行：
  - `alertflow.registry.events.v1`
  - `alertflow.monitor.events.v1`
  - `alertflow.episode.events.v1`
  - `alertflow.task.events.v1`
- **过渡 topic**：旧 `alertflow.system.events.v1` 在 P1~P5 保留为桥接 topic，P6 收口后停用。

手工重建 topic：

```bash
docker compose -f deploy/compose/compose.yaml run --rm kafka-init
```

## 域账号隔离（setup profile）

```bash
docker compose -f deploy/compose/compose.yaml --profile setup run --rm provision
```

该步骤幂等，创建：

| schema | role | 默认本地口令（env 覆盖） |
|---|---|---|
| `registry` | `alert_registry` | `ALERT_REGISTRY_PASSWORD` |
| `monitor` | `alert_monitor` | `ALERT_MONITOR_PASSWORD` |
| `episode` | `alert_episode` | `ALERT_EPISODE_PASSWORD` |
| `task` | `alert_task` | `ALERT_TASK_PASSWORD` |

隔离规则：`REVOKE ALL ON SCHEMA public FROM PUBLIC`；每个角色只被授予自有 schema 的
`USAGE/CREATE` 与对象权限，跨 schema 访问在数据库层被拒绝。`system` / `cognitive` schema 不动。

反向验证（跨 schema 读写必须被拒）：

```bash
bash deploy/split/verify-isolation.sh
```

## 已知限制

- 宿主机上的 Go 进程必须使用 `127.0.0.1:9092`；容器内进程必须使用 `kafka:29092`。混用会拿到无法路由的 advertised 地址。
- 示例口令 `alert/alert` 与各域默认口令仅限本地使用，非真实凭据。