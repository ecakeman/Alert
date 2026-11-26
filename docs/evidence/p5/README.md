# P5 阶段进展与验收证据（进行中）

## 1. 已完成 task（8/11；P5.9 接线完成、执行链受阻）

| task | 提交 | 验收要点 |
|---|---|---|
| P5.1 module/迁移 | `a0278e7` | 6 表、跨 schema FK=0、部分唯一索引、`applied=1→skipped=1`、镜像构建 PASS |
| P5.2 四表搬迁 | `e649554` | `tasks=12 ownership=8 artifacts=3 docs=0`，二次 no-op |
| P5.3 用例迁移 | `24903e0` | 真 PG：创建/静默幂等/同类型复用/冲突 |
| P5.4 gRPC :9090 | `f013119` | 7 RPC 有响应、旧代次 FailedPrecondition、端口 fail-fast |
| P5.5 派发消费者 | `c161555` | dispatchable→1 任务；重放 3 次仍 1；agent 不可达→RETRY_WAIT |
| P5.6 产物/重试 API | `c161555` | 列表/正文 200；retry 202 且 generation=2 |
| P5.7 reconciler | `679296d` | 5s 周期；过期租约不再永久 RUNNING；事件丢失对账补建 1 |
| P5.8 BFF 切 task + compose | `6979e18` | route16=200、route17=202（generation 递增）；task 容器无宿主端口；compose.p5 config OK |

## 2. 本轮修复的缺陷

1. `UpdateCAS` 对过期租约恒不命中，导致任务永久 RUNNING → 增加 `AllowExpiredLease`（回收路径专用）。
2. `Ownership.Release` 无行时返回 NotFound，阻断回收 → 回收路径按幂等处理。
3. 业务冲突（STATE_CONFLICT）被消费者当作可重试错误 → 改为已判定即提交，避免毒消息阻塞分区。
4. gen 包 raw descriptor 不可做文本替换（会破坏长度前缀）→ gen 目录原样复制，仅改 import 行。

## 3. 未完成（P5.8~P5.11）

| task | 状态 |
|---|---|
| P5.9 认知栈接线 | **接线完成**：`python/agent-runtime/Dockerfile` 与 compose 服务已交付；宿主 venv 真实启动认知栈（50052/18080），task-orchestrator 容器可向其提交、认知栈可回调 task-orchestrator（实测收到历史任务并进入 recover）。修复认知侧 `TaskRunner` 的 `self._store/_clock/_executor` 属性名缺陷。**执行链受阻**：认知侧报 `lease token is not persisted; needs re-dispatch`（既有缺陷），无法完成“回调成功 + 产物”验收 |
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

## 7. P5.9 的构建记录与后续

- 已交付：`python/agent-runtime/Dockerfile`、compose `agent-runtime` 服务、Go 端默认上游 `50052`。
- 未完成：`docker build -t alert/agent-runtime:p5 python/agent-runtime` 在依赖下载阶段超时（grpcio/grpcio-tools/zstandard），未产出镜像。
- 后续动作（下一次长构建窗口）：
  1. 预热 pip/uv 缓存或改用国内镜像源；
  2. 镜像构建完成后 `compose up agent-runtime`，验证 `task-orchestrator:9090` 可被认知栈回调、`agent-runtime:50052` 可被 Go SubmitTask 调用；
  3. 再执行 P5.10（单体下线/全路由）与 P5.11（全链路门槛）。

**未以 stub/跳过替代**：P5.9 的运行验证与 P5.10/P5.11 的全链路验证尚未完成。

## 8. P5.9 阻塞点（认知侧既有缺陷）

1. 已修：`src/alert_agent/bootstrap/worker.py` 的 `self._store / self._clock / self._executor` 与 dataclass 字段 `store / clock / executor` 不一致，导致 TaskRunner 崩溃。
2. 未修（阻塞完整链路）：认知侧恢复扫描输出 `cannot renew: lease token is not persisted; needs re-dispatch` —— 任务受理时未持久化 `lease_token`，导致恢复/续租失败。需在认知侧 receipt 存储增加 lease_token 持久化并衔接 Go 的围栏校验。
3. 镜像构建：容器网络无法访问 PyPI/镜像源（`No matching distribution found for uv`），容器化交付物就绪但未产出镜像；当前用宿主 venv 做真实运行验证。

在 2 修复前，P5.9 的“回调成功”与 P5.10/P5.11 全链路不能标记通过。

## 9. P5.9 复验记录（2026-11-26）

已完成的修复：
1. 认知侧 `TaskRunner` 属性名（`self._store/_clock/_executor`）。
2. task 侧派发/重试 CAS 增加 `AllowExpiredLease`，允许过期租约重新派发。
3. 认知栈以宿主 venv 真实运行（50052/18080），task-orchestrator 临时映射 `9090` 供宿主回调。

已经验证：
- dispatchable 事件 → task-orchestrator 建任务（PENDING）→ reconcile 派发（DISPATCHED）→ Submit 被认知栈受理（agent 收到任务）。
- 宿主 `:9090` 可达（映射生效）；认知栈 `:50052` 监听正常。

仍未闭环：
- DISPATCHED 任务在 `Ownership.Acquire` 处被 skip（`CanAcquire` 规则本身允许同任务/过期租约），回调 `ReportTaskStarted` 未成功；需检查 `ownership_repo.Acquire` 的 SQL 条件（generation/lease 判定）。
- 因此 P5.9 的“回调成功 + 产物”与 P5.10/P5.11 仍未标记通过。

注：临时 `9090` 宿主映射仅用于认知栈未容器化期间的回调验证，认知栈容器化后应移除（P5.8 的“无宿主端口”目标不变）。
