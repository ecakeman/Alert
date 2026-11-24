# S0 基线冻结（结构收敛前）

- 生成日期：2026-11-24
- 开发根：`/home/sancho/workspace/projects/Alert`
- 声明：**本文件为整改前基线快照，不代表当前通过状态；后续结构收敛后以此对照回归。**

## 1. 记录

| 项 | 值 |
|---|---|
| HEAD | `5c6dc8636f697a636ec6701a20f7a707bc6f763d` |
| 工作区 | `git status --short` 为空（干净） |
| Go | `go1.27.4 linux/amd64` |
| Node / npm | `v22.23.1` / `10.9.8` |

## 2. 目录与文件快照

`git ls-files cmd internal migrations contracts | wc -l` = **111**。

两级目录：

```
cmd/system-core
contracts/{openapi,proto}
internal/{application,bootstrap,config,domain,infrastructure,transport}
internal/application/{apperr,dashboard,episode,health,monitor,outbox,port,service}
internal/infrastructure/{messaging,persistence,probe,telemetry}
internal/transport/http
migrations
```

## 3. 构建与静态检查（整改前）

| 检查 | 命令 | 结果 |
|---|---|---|
| build | `go build ./...` | 通过（exit 0） |
| vet | `go vet ./...` | 通过（exit 0） |

## 4. 迁移文件 SHA-256

```
79c451171f137e8263465b3f11ef39e9f494c2df63a607c883da428099fd9662  migrations/0001_system_core_entities.sql
9c721b45c514ceaca5ce5e51d2f543d759855f0f66208d1564ea4d21e1b55454  migrations/0002_input_events_points.sql
3bc5f8e9c4b91a146396b40e287418acf73b363d7b14d6835e0d9812f5913ee9  migrations/0003_tasks_outbox_artifacts.sql
```

## 5. B7 保留清单（不删除、不接线、不宣称完成）

- domain：`task.go`、`artifact.go`、`bound.go`、`state.go` + 对应 `_test.go`（语义不动）
- 仓储：`tasks_repo.go`、`ownership_repo.go`、`artifacts_repo.go`、`idempotency_repo.go`
- 端口：`TaskRepo/OwnershipRepo/ArtifactRepo/TaskDispatcher`
- 路由：`/episodes/:id/artifacts`、`/episodes/:id/retry-diagnosis`、`/episodes/:id/retry-archive`（保持 501）
- 配置：`AGENT_GRPC_TARGET`、`GRPC_ADDR`、`TASK_*`、`OWNERSHIP_TTL`（保留解析，不新增接线）

## 6. 路由现状（router.go）

**真实路由（15 条）+ 探针（2 条）**：

- 探针：`GET /healthz`、`GET /readyz`
- 1 `GET /api/v1/dashboard/summary`
- 2 `GET /api/v1/services`
- 3 `POST /api/v1/services`
- 4 `GET /api/v1/services/:service_id`
- 5 `PATCH /api/v1/services/:service_id`
- 6 `DELETE /api/v1/services/:service_id`
- 7 `GET /api/v1/services/:service_id/targets`
- 8 `POST /api/v1/services/:service_id/targets`
- 9 `PATCH /api/v1/targets/:target_id`
- 10 `DELETE /api/v1/targets/:target_id`
- 11 `GET /api/v1/episodes`
- 12 `GET /api/v1/services/:service_id/timeline`
- 13 `POST /api/v1/services/:service_id/events`
- 14 `GET /api/v1/episodes/:episode_id`
- 15 `GET /api/v1/episodes/:episode_id/timeline`

**501（pendingRoutes，3 条）**：

- `GET /api/v1/episodes/:episode_id/artifacts`
- `POST /api/v1/episodes/:episode_id/retry-diagnosis`
- `POST /api/v1/episodes/:episode_id/retry-archive`

## 7. 配置项

- `internal/config/config.go` 的 `Config` 结构共 **33 项**（既有 7 项 + 扩展 26 项），默认值在 `Load` 内，`Validate`/`ValidateRuntime` 校验；`DATABASE_URL`、`KAFKA_BROKERS`（publisher 启用时）、`AGENT_GRPC_TARGET`（B7）无默认。
- `.env.example` 33 个键与之对应（示例值仅本地）。

## 8. 前端

`apps/console` 已移植，B4 阶段 `npm run build` 通过（16.45s）；本 task 未重跑前端。

## 9. 结论

本文件固定整改前事实，供 S3 搬迁与 S5 回归对照。任何未在本文档记录的状态变化，均需在对应 task 的证据中补充。