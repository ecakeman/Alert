# 契约

## 事件（Kafka，信封 v1）
4 个按域 topic，3 分区 / 1 副本，由 `kafka-init` 幂等创建：

- `alertflow.registry.events.v1`
- `alertflow.monitor.events.v1`
- `alertflow.episode.events.v1`
- `alertflow.task.events.v1`

信封与强类型载荷定义在 `go/contracts`（事件类型、schema 版本、payload DTO、trace 键）。消费端以
`consumer_idempotency`（键含 event_id）去重，未支持的类型显式失败、不标记完成。

## HTTP 两层
- **L1 对外（18 路由）**：`console-bff` 反代 + 聚合，路径 `/api/v1/*`；输出为 P0 冻结契约（列表/详情/时间轴/dashboard）。
- **L2 域内**：各域 `/internal/*`，仅 BFF/域间调用；`registry`/`episode`/`task` 的批量端点上限 200。

## 错误信封（统一）
```json
{ "error": { "code": "NOT_FOUND", "message": "resource not found", "request_id": "<hex32>" } }
```
- 稳定码：VALIDATION(422)/NOT_FOUND(404)/ALREADY_EXISTS(409)/STATE_CONFLICT(409)/DEPENDENCY_UNAVAILABLE(503)/NOT_IMPLEMENTED(501)/INTERNAL(500)。
- 上游域返回的扁平错误由 BFF 归一为上述嵌套信封；上游 404 透传为 404（不误报 503）。