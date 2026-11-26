# 契约边界（contracts）

> 语言无关契约源在根 `contracts/`；生成物与内部模型各归其主。

## 1. 事件信封（唯一线上契约）

- 源：`contracts/events/event-envelope.v1.schema.json` + 同目录 README（8 个公开字段、3 个事件类型、兼容规则）。
- Go 侧类型：`services/system-core/internal/contracts/`，负责构造/编码/发布/解码；与 Schema 同源，不反向回写。
- Python 侧（B8 未实现）：基于 Schema 生成/校验消费模型。
- 投递控制字段（claim/ack/重试）只存在于 Outbox 存储行，**不进入线上信封**。

## 2. proto/gRPC（预留）

- 包路径约定：`alertflow/agent/v1/`、`alertflow/system/v1/`；接线前不建空子目录。
- 生成物归属：Go → `services/system-core`（接线后在 transport/grpc）；Python → `python/agent-runtime/gen/`。
- 当前状态：未生成任何 proto 源与生成物。详见 `contracts/proto/README.md`。

## 3. 公开 REST（openapi 说明）

- 18 路由 + 探针，注册唯一入口 `services/system-core/internal/transport/http/router/`。
- 1~15 真实实现；16~18（artifacts / retry-diagnosis / retry-archive）保持 501（B7 待接线）。
- DTO JSON 字段以 `internal/transport/http/dto/` 的 `json` 标签为事实来源；本目录文档不另起版本。
- 详见 `contracts/openapi/README.md`。

## 4. 通用纪律

1. 契约只增可选字段；破坏性变更递增 `schema_version` 并保留旧版本。
2. 消费者对未知 `event_type` / `schema_version` 显式失败，不得静默标记完成。
3. 契约文档不得引用任一侧内部类型；冲突时以代码/Schema 为准并回改文档。