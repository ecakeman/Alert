# contracts/openapi

公开 REST 契约说明目录。当前**不另起 OpenAPI 版本、不生成 openapi.yaml**，以既有路由说明为准。

## 当前公开 REST 面（18 路由 + 探针）

- 路由 1~15：真实实现，见 `docs/evidence/alert/u07-b4.md` 的「路由状态表」。
- 路由 16~18（`/episodes/:id/artifacts`、`/episodes/:id/retry-diagnosis`、`/episodes/:id/retry-archive`）：保留 501，属 B7 待接线。
- 探针：`/healthz`、`/readyz`；未注册路径返回统一 404。
- 路由注册唯一入口：`go/services/system-core/internal/transport/http/router/`。

## 约定

1. 本目录只描述公开 HTTP 契约（方法/路径/状态码/DTO 形状），不引用任何 Go internal 类型。
2. DTO 的 JSON 字段以 `go/services/system-core/internal/transport/http/dto/` 的 `json` 标签为唯一事实来源；本目录文档与之冲突时以后者为准。
3. 新增路由时先在 router 注册，再同步本目录说明；不做独立的 OpenAPI 版本分支。