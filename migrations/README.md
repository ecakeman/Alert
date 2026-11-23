# migrations

forward-only 迁移目录（runner 自动发现并按版本升序执行）。

## 命名约定

- 文件名：`NNNN_snake_case_name.sql`（四位版本号 + 下划线小写名，正则 `^(\d{4})_([a-z0-9_]+)\.sql$`）
- 内容：纯 SQL，**不含** `BEGIN/COMMIT`（由 runner 单文件单事务包裹）、禁止 `DROP`、禁止 `CREATE SCHEMA`（schema 由 runner 引导）
- 已应用文件不得修改（checksum 校验拒绝）；不支持 down

## 版本表

`system.schema_migrations`（version, name, checksum, applied_at, execution_ms），由 runner 幂等引导。