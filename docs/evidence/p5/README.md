# P5 阶段进展与验收证据（进行中）

## 1. 已完成 task（8/11）

| task | 提交 | 验收要点 |
|---|---|---|
| P5.1 module/迁移 | `a0278e7` | 6 表、跨 schema FK=0、部分唯一索引、`applied=1→skipped=1`、镜像构建 PASS |
| P5.2 四表搬迁 | `e649554` | `tasks=12 ownership=8 artifacts=3 docs=0`，二次 no-op |
| P5.3 用例迁移 | `24903e0` | 真 PG：创建/静默幂等/同类型复用/冲突 |
| P5.4 gRPC :9090 | `f013119` | 7 RPC 有响应、旧代次 FailedPrecondition、端口 fail-fast |
| P5.5 派发消费者 | `c161555` | dispatchable→1 任务；重放 3 次仍 1；agent 不可达→RETRY_WAIT |
| P5.6 产物/重试 API | `c161555` | 列表/正文 200；retry 202 且 generation=2 |
| P5.7 reconciler | `679296d` | 5s 周期；过期租约不再永久 RUNNING；事件丢失对账补建 1 |
| P5.8 BFF 切 task + compose | 本提交 | route16=200、route17=202（generation 递增）；task 容器无宿主端口；compose.p5 config OK |

## 2. 本轮修复的缺陷

1. `UpdateCAS` 对过期租约恒不命中，导致任务永久 RUNNING → 增加 `AllowExpiredLease`（回收路径专用）。
2. `Ownership.Release` 无行时返回 NotFound，阻断回收 → 回收路径按幂等处理。
3. 业务冲突（STATE_CONFLICT）被消费者当作可重试错误 → 改为已判定即提交，避免毒消息阻塞分区。
4. gen 包 raw descriptor 不可做文本替换（会破坏长度前缀）→ gen 目录原样复制，仅改 import 行。

## 3. 未完成（P5.8~P5.11）

| task | 状态 |
|---|---|
| P5.9 认知栈接线 | 未开始（agent-runtime 尚未容器化，需先补 Dockerfile + compose 服务） |
| P5.10 单体下线 | 部分（单体 api/worker 已停；BFF legacy 仍指 8084，dashboard/timeline 待 P6 聚合） |
| P5.11 全链路门槛 | 未开始（依赖 P5.8~P5.10） |

## 4. 未实证项（需 P5.11）

- attempt 上限 → FAILED 的完整路径（当前环境认知栈不可达，重试循环停在 DISPATCHED/RETRY_WAIT）。
- 认知栈可达时的回调成功（grpc → task → 产物 → 控制台）。

## 5. 运行态现状

- task-orchestrator：HTTP :8084 / gRPC :9090（宿主进程）；episode-svc 容器 healthy。
- BFF：registry → :8081、monitor → :8083、episode → episode-svc:8082；task/legacy 仍指宿主 :8084（P5.8 切换）。

## 6. P5.9 的硬依赖

`python/agent-runtime` 目前无 Dockerfile，compose 中无 agent-runtime 服务。P5.9 必须新增：

1. `python/agent-runtime/Dockerfile`（Python 3.12 + 依赖安装 + 启动 `alert_agent.bootstrap.app`）；
2. compose 服务 `agent-runtime`（`HTTP_LISTEN` / `AGENT_GRPC_LISTEN` 不变，`AGENT_GRPC_TARGET=task-orchestrator:9090`）；
3. 认知执行所需模型凭证（`.env`）。

未完成此三项则 P5.10/P5.11 的全链路无法真实跑通；不得以 stub 代替。
