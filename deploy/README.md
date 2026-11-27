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
- **过渡 topic（P6 起已停用）**：旧 `alertflow.system.events.v1` 数据仅作历史，消费者组为空、无新增生产；`alertflow.system.events` 在 `go/`、`python/`、`deploy/compose/` 配置中零命中。
- **编排快照**：`compose.p1~p5.yaml` 为历史快照，**不用于运行**；最终态由 `compose.yaml` 的 core profile 提供 **9 个运行容器**（postgres、kafka、console-bff、service-registry、monitor-svc、episode-svc、task-orchestrator、agent-runtime 共 8 个常驻 healthy + `kafka-init` 一次性 exited 0）。

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

断言共 8 条，输出 `PASS n/8`：

- 反向 4 条：`alert_registry` 读 `monitor`、`alert_monitor` 写 `registry`、`alert_episode` 读 `task`、`alert_task` 写 `episode`，必须返回 `permission denied`；
- 正向 4 条：各角色对自有 schema 中由 `alert` 创建的表执行 `INSERT/SELECT`（覆盖 DEFAULT PRIVILEGES 与序列），并自行建表验证 `CREATE` 权限。

任何一条不符合预期即非零退出。

## P0 阶段门槛

```bash
GO_BIN=/home/sancho/go-dist/go/bin/go bash scripts/verify-p0.sh
```

串行检查构建/vet、contracts 与 platform 单测、module 边界、provision、账号隔离、S0 预检、
迁移幂等与 18 路由快照，输出 `PASS/FAIL/SKIP` 三态，任一 FAIL 非零退出。
快照项需要 `历史单体（已下线）` API 在 `127.0.0.1:8080` 运行；证据与补跑方式见
`docs/evidence/p0/README.md`。

## 已知限制

- 宿主机上的 Go 进程必须使用 `127.0.0.1:9092`；容器内进程必须使用 `kafka:29092`。混用会拿到无法路由的 advertised 地址。
- 示例口令 `alert/alert` 与各域默认口令仅限本地使用，非真实凭据。