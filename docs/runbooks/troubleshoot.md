# Runbook：排障

## 1. 迁移发现异常

- 现象：`-migrate` 报版本缺口/重复/checksum conflict。
- 排查：`ls go/services/system-core/migrations/*.sql`（工作目录 `go/services/system-core`）确认只有 3 个 SQL 且文件名未改；对照 `git log -- migrations/`。
- 纪律：不手改 schema_migrations 记录；恢复文件原文后重放。

## 2. 旧 import 前缀（S3 搬迁后）

```bash
rg -n 'github.com/ecakeman/alert/internal|github.com/ecakeman/alert/migrations' go/services/system-core
```

期望无输出；有输出说明存在未适配的旧前缀引用。

## 3. 双进程误配

- api 进程出现 probe/outbox/consumer 日志、或 worker 进程占用 8080 → 装配越界。
- 核对 `internal/bootstrap/api.go` 与 `worker.go` 的白名单注释与装配项；api 不启后台循环，worker 不启 HTTP。

## 4. 看板空态与失败区分

- `GET /api/v1/dashboard/summary` 返回空 `recent`/`trend`（200）＝无数据；返回 5xx ＝查询链路失败。
- 失败时不重试到空数组：查 api 日志中 `apperr` 映射（依赖不可用类）与 PG 连接池健康。

## 5. 事件链路无产出

- 依次确认：业务写事务产生 `outbox_events` 行 → worker 的 publisher 启用（`OUTBOX_PUBLISHER_ENABLED`）→ Kafka broker 可达 → topic `alertflow.system.events.v1`。
- consumer 幂等键为 `event_id`；重复消费不产生双写。