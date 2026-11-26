# python/agent-runtime（Python 认知执行栈）

Alert 的 AI Agent 运行时：一个进程内同时承载 **HTTP/gRPC 网关、诊断 Agent、RAG Agent 与摄取 worker**，接收 Go 控制面派发的任务，产出可追溯的诊断产物与可检索的故障知识。

## 能力总览

| 能力 | 说明 |
|---|---|
| 诊断 Agent | langgraph 受约束 loop：拉取冻结范围内时间轴 → 检索历史故障 → 咨询知识智能体（A2A）→ 调用 MCP 工具 → 提交报告；预算耗尽时显式产出"未完成"结论（禁止编造答案） |
| 双产物沉淀 | 调查记录（Markdown，按轮次追加 round marker）与诊断报告（JSON）带版本号原子写盘，经乐观锁登记并回调 Go |
| RAG 检索问答 | 向量（pgvector）+ BM25 双腿检索、RRF 融合、可降级重排；`POST /chat` 真流式 SSE（trace/token/citations/done/error 五类事件），引用编号可溯 |
| 文档摄取 | `POST /ingest` 受理（路径与回调地址安全校验）→ 持久化 job → 单事务写入父子块与向量 → PROCESSING/READY/FAILED 三态回调 |
| 归档索引 | ARCHIVE 任务从诊断产物提取四字段故障索引（symptoms ≠ observations 强校验），支撑跨 episode 故障检索 |
| 协议面 | MCP Streamable-HTTP 长会话客户端；A2A AgentCard 与 message/send 端点 |

## 架构

五层骨架（`src/alert_agent/`），依赖单向向下：

```
domain/          纯规则与 pydantic 模型（无 IO、无框架）
application/     用例与 port 协议（依赖倒置，无技术 import）
infrastructure/  技术实现：psycopg 仓储 / OpenAI 兼容 LLM / MCP / A2A / pgvector / BM25
transport/       gRPC servicer、HTTP handler/DTO/mapper、SSE 编码（薄）
bootstrap/       装配、lifespan、worker 循环、迁移入口
```

- 认知数据独占 `cognitive` schema（5 个 forward-only 迁移），与 Go 的 `system` schema 严格隔离。
- 所有配置由单一 `pydantic-settings` 入口声明，缺失/占位值启动即 fail-fast；LLM / Embedding / Rerank 端点未配置时对应能力显式不可用，其余链路独立运行。

## 快速开始

```bash
cd python/agent-runtime
python3 -m venv --without-pip .venv
curl -sS https://bootstrap.pypa.io/get-pip.py -o /tmp/get-pip.py
.venv/bin/python /tmp/get-pip.py
.venv/bin/pip install -e '.[dev]'

cp .env.example .env        # 修改 DATABASE_URL_COGNITIVE 等；LLM 端点可暂不配置
.venv/bin/python -m alert_agent.bootstrap.migrate    # cognitive schema 迁移
.venv/bin/python -m alert_agent.bootstrap.app        # 启动（HTTP :8080 + gRPC :50052）
```

冒烟：`curl http://localhost:8080/health`；带内部 token 访问 `/search`、`/chat`。

## 测试与评测

```bash
.venv/bin/ruff check src eval     # 静态检查
.venv/bin/pytest tests -q         # 定向单测（不设覆盖率指标）
.venv/bin/python eval/run.py --dataset eval/datasets/simple_qa.csv   # 离线检索评测（需真实环境）
```

## proto 生成物

`infrastructure/grpc/gen/` 由根 `contracts/proto/` 生成，禁止手工编辑：

```bash
.venv/bin/python tools/generate_proto.py
```

## 与 system 侧的边界

| 项 | system（Go） | cognitive（本组件） |
|---|---|---|
| 迁移目录 | `go/services/system-core/migrations/` | `python/agent-runtime/migrations/` |
| 连接变量 | `DATABASE_URL` | `DATABASE_URL_COGNITIVE` |
| 数据所有权 | `system` schema 只读保护 | 独占写 `cognitive` schema，不触碰 system |

产物正文只存文件（`storage_key` 地址制），数据库与回执不承载正文；Go 侧经 `ARTIFACT_READER_ROOT` 指向共享存储目录读取。