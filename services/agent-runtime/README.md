# services/agent-runtime（Python 组件占位）

> 当前状态：**仅占位**。本目录只声明 Python 组件边界，不含任何 Python 源码、迁移或脚手架。

## 范围声明

- 保留清单 B8（Agent Runtime / cognitive 侧）**未实现**：无业务代码、无迁移、无依赖清单条目、无启动入口。
- 本组件未来只消费 `contracts/events/event-envelope.v1.schema.json` 定义的事件信封，不共享 system 侧的 Schema 写权限。

## 未来启动方式（设计约定，当前无对应实现）

- 组件根：`services/agent-runtime/`；Python 依赖与虚拟环境由该目录自行管理（`pyproject.toml` 为最小组件声明）。
- 认知库迁移将来位于本目录 `migrations/`，由 B8 的 Python 迁移入口回放，与 Go 侧 runner 完全隔离。

## 与 system 侧的边界

| 项 | system（Go，已落地） | cognitive（Python，B8 未实现） |
|---|---|---|
| 迁移目录 | `services/system-core/migrations/` | `services/agent-runtime/migrations/`（B8 落点，当前不建目录） |
| 连接变量 | `DATABASE_URL` | `DATABASE_URL_COGNITIVE`（B8 引入，当前**不新增**该变量） |
| 回放入口 | `go run ./cmd/api -migrate` | B8 的 Python 迁移入口（未存在） |

- 禁止两侧共用写账号；禁止 Go runner 发现 Python 迁移；禁止根级公共迁移池。
- 在 B8 接线前，本目录除本 README 与 `pyproject.toml` 外不应出现任何文件。