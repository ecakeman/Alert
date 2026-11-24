# contracts/proto

Protobuf/gRPC 契约预留目录（对应保留清单 B7 / gRPC 接线），**本次不生成任何代码、不创建子目录**。

## 包路径约定

| 包路径 | 用途 | 消费方 |
|---|---|---|
| `alertflow/agent/v1/` | 控制面调用 Agent Runtime 的服务定义 | Go（system-core 侧客户端） |
| `alertflow/system/v1/` | 事件/任务相关的 system 服务定义 | Go（system-core 侧服务端） |

- `.proto` 源文件将来落在本目录对应包路径下；两个 v1 子目录在接线前**不建空目录**。

## 生成物归属（主从关系）

| 语言 | 生成物落点 | 说明 |
|---|---|---|
| Go | `services/system-core/internal/transport/grpc/`（接线后） | 生成 `.pb.go`，与 `services/system-core` 同 module |
| Python | `services/agent-runtime/gen/`（B8 接线后） | 生成 `_pb2.py`，与 `services/agent-runtime` 同组件 |

- 契约源在本目录，生成物**不得跨语言复制**、不得反向回写成契约源。
- 当前状态：**本次未生成**任何 proto 源文件与生成物；接线前本目录仅有本 README。