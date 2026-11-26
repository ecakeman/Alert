# 数据所有权（data-ownership）

> system（Go）与 cognitive（Python）的 Schema、迁移与账号边界。禁止共享写权限。

## 1. 所有者对照

| 所有者 | 迁移目录 | 连接变量 | 回放入口 | 当前状态 |
|---|---|---|---|---|
| system（Go） | `services/system-core/migrations/` | `DATABASE_URL` | `go run ./cmd/api -migrate`（工作目录 `services/system-core`） | 已落地（3 个版本，forward-only） |
| cognitive（Python） | `python/agent-runtime/migrations/` | `DATABASE_URL_COGNITIVE`（B8 引入） | B8 的 Python 迁移入口 | 未实现；不建目录、不建 SQL、不引入变量 |

## 2. 边界规则

1. 禁止根级公共迁移池；各组件迁移只在自己的 `migrations/` 目录内发现。
2. 禁止 Go runner 发现或回放 Python 迁移；反之亦然。
3. 两侧禁止共用写账号：system 用 `DATABASE_URL` 指向的账号只授权 system schema；cognitive 未来账号只授权 cognitive schema。
4. 共享 `contracts/`（事件信封）不改变 Schema 所有权。
5. system schema 表结构变更只能走新增迁移文件；不改既有 SQL 正文、文件名与版本号。

## 3. 迁移文件事实

- 文件：`0001_system_core_entities.sql`、`0002_input_events_points.sql`、`0003_tasks_outbox_artifacts.sql`。
- 版本 1/2/3；名称取文件名段；checksum 为文件内容 SHA-256；重复回放 no-op。
- `embed.go` 仅嵌入 `*.sql`。