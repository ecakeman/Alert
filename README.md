# Alert

面向服务故障的**监控、调查与可追溯诊断**系统：当探测发现服务异常时，系统自动立案（episode）、派发诊断任务给认知执行栈（AI Agent），并把调查过程与结论沉淀为可追溯的产物与可检索的故障知识库。

## 项目组成

| 组件 | 技术栈 | 职责 |
|---|---|---|
| `services/system-core/` | Go（Gin + pgx + Kafka） | **控制面**：监控探测（HTTP/TCP）、告警聚合与立案、任务派发与围栏（fence）、产物登记与读取、REST API（18 条路由） |
| `services/agent-runtime/` | Python（FastAPI + langgraph + psycopg） | **认知执行栈**：诊断 Agent（受约束 loop，产出调查记录与诊断报告双产物）、RAG Agent（双腿检索 + 流式问答 + 文档摄取）、MCP/A2A 协议面 |
| `apps/console/` | Vue 3 | **控制台**：服务/告警/任务的可视化操作界面 |
| `contracts/` | proto / JSON Schema | 语言无关契约（gRPC 任务派发与回调、事件信封） |

## 工作方式

1. **探测与立案**：worker 进程对服务目标执行 HTTP/TCP 探测，阈值聚合后创建故障事件（episode）。
2. **任务派发**：控制面通过 gRPC 把诊断/归档任务（含冻结的输入版本范围与截止时间）派发给认知执行栈，心跳与租约保证不双执行。
3. **诊断执行**：认知侧的诊断 Agent 在冻结边界内循环执行「拉时间轴 → 检索历史故障 → 咨询知识库 → 调用 MCP 工具 → 提交报告」，预算耗尽时显式产出"未完成"结论而非编造答案。
4. **产物沉淀**：调查记录（Markdown）与诊断报告（JSON）带版本号落盘并登记回控制面，正文可被 Go 侧读取。
5. **知识沉淀**：归档任务从双产物提取故障索引落库；RAG 侧支持向量 + BM25 双腿检索、引用可溯的流式问答（SSE）与文档摄取。

## 快速开始

```bash
# 1. 依赖拓扑（PG 需 pgvector 扩展，compose 已配置）
docker compose -f deploy/compose/compose.yaml up -d

# 2. 控制面（Go）
cd services/system-core
go run ./cmd/api -migrate          # 首次迁移（system schema）
go run ./cmd/api                   # REST 进程
go run ./cmd/worker                # 后台进程（探测 / 派发 / 消费）

# 3. 认知执行栈（Python）
cd ../agent-runtime
cp .env.example .env               # 按需修改数据库与 LLM 端点
.venv/bin/python -m alert_agent.bootstrap.migrate   # 首次迁移（cognitive schema）
.venv/bin/python -m alert_agent.bootstrap.app       # 网关 + 双 Agent + worker 一体进程
```

构建验证：`go build ./... && go vet ./...`（Go）；`.venv/bin/ruff check src eval`（Python）。

## 对外接口一览

- **控制面 REST**（`:8080`，Go）：服务/目标/告警事件/任务的管理与产物读取（路由 1~18）。
- **认知面 HTTP**（`:8080`，FastAPI）：
  - `POST /chat`（SSE 流式问答，trace/token/citations/done/error 五类事件）
  - `POST /search`、`POST /ingest`、`GET /ingest/{docId}/status`、`DELETE /ingest/{docId}`（文档摄取）
  - `GET /.well-known/agent.json` 与 `POST /a2a/message/send`（A2A 协议面）
- **任务通道**（gRPC）：`AgentTaskService.SubmitTask`（派发）与 `SystemTaskGatewayService`（7 类回调）。

认知面与控制面的管理接口均要求内部 token（`X-Internal-Token`，常量时间比较）。

## 目录导航

| 目录 | 内容 |
|---|---|
| `services/system-core/` | Go 控制面（api + worker 双入口、六层结构、迁移） |
| `services/agent-runtime/` | Python 认知执行栈（五层骨架 + 迁移 + eval） |
| `apps/console/` | Vue 控制台 |
| `contracts/` | proto 与事件 Schema（契约源，生成物可随时重建） |
| `docs/` | `architecture/` 架构说明、`runbooks/` 操作手册、`evidence/` 验收记录 |
| `deploy/compose/` | 本地依赖拓扑（pgvector 版 PG + Kafka KRaft） |

## 测试与验收

```bash
cd services/system-core && go build ./... && go vet ./... && go test ./internal/domain/... ./internal/application/...
cd services/agent-runtime && .venv/bin/ruff check src eval && .venv/bin/pytest tests -q
```

离线检索评测（可选）：`services/agent-runtime/eval/run.py`，用法见该目录 README 段落。

## 已知限制

1. 仅面向本地 / 可信内网演示：控制台与认知面未接入终端用户鉴权。
2. 无 K8s 编排与微服务拆分：Go 为单 module 双进程，Python 为单进程一体栈。
3. 诊断与向量检索依赖外部 LLM / Embedding 端点；未配置时相关能力显式不可用（配置校验 fail-fast），其余链路可独立运行。