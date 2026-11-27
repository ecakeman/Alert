# 迁移

全部 forward-only。迁移是**各域自管**，不使用单一全局入口。

## 1. 域账号与 schema（provision，setup profile）

```bash
docker compose -f deploy/compose/compose.yaml --profile setup run --rm provision
```

创建 4 schema / 4 role：`registry→alert_registry`、`monitor→alert_monitor`、`episode→alert_episode`、`task→alert_task`（口令见 `.env.example`）。

## 2. 四个 Go 域（各自 `-migrate`）

```bash
for s in service-registry monitor-svc episode-svc task-orchestrator; do
  docker compose -f deploy/compose/compose.yaml run --rm "$s" -migrate
done
```

输出 `applied=N skipped=M conflicts=K`；forward-only，不提供 down。

## 3. 认知 schema（Python agent-runtime）

```bash
docker compose -f deploy/compose/compose.yaml run --rm agent-runtime python -m alert_agent.bootstrap.migrate
```

对 `DATABASE_URL_COGNITIVE` 执行 `python/agent-runtime/migrations/*.sql`（镜像已 `COPY migrations`，路径 `/app/migrations`）。