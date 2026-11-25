# services/agent-runtime（Python 认知执行栈）

Alert 认知侧：FastAPI 网关 + 诊断 agent + RAG agent，单包 `src/alert_agent/` 五层骨架（domain / application / infrastructure / transport / bootstrap，design §2.1）。

## 当前状态（真实范围，不宣称未实现能力）

已落地（T01 骨架批次）：

- `src/alert_agent/` 包骨架与 `infrastructure/grpc/gen/` 生成物：由根 `contracts/proto/` 三个 proto（`agent/v1/agent_task.proto`、`system/v1/common.proto`、`system/v1/task_gateway.proto`）生成，生成后 import 统一改写为 `alert_agent.infrastructure.grpc.gen.*`；本目录不复制 proto 源文件（CR-GW-01）。
- 依赖版本锁定：`pyproject.toml`；`grpcio` / `grpcio-tools` / `protobuf` 锁定同版本族（grpcio 1.62.x + protobuf 4.25.x），生成物与运行时版本自洽（CR-REG-48）。
- `tools/generate_proto.py`：proto 生成物重生成脚本，脚本内记录生成时的 grpcio-tools 版本（1.62.3）并校验 protobuf 4.25 族。
- `.env.example` 占位模板（仅键与占位值，不含实值，CR-BOOT-03）。

未落地（随 T02~T24 逐 task 落地）：集中配置 Settings、双 server lifespan、cognitive schema 迁移与仓储、任务链（receipt/worker/心跳/恢复）、诊断 agent、归档、RAG 检索与摄取、MCP/A2A、SSE、观测、eval。

## 构建与虚拟环境

```bash
cd services/agent-runtime
python3 -m venv --without-pip .venv
curl -sS https://bootstrap.pypa.io/get-pip.py -o /tmp/get-pip.py
.venv/bin/python /tmp/get-pip.py
.venv/bin/pip install -e '.[dev]'
```

## 重新生成 proto 生成物

```bash
.venv/bin/python tools/generate_proto.py
```

- 生成物只放 `src/alert_agent/infrastructure/grpc/gen/`，可随时由根 `contracts/proto/` 重新生成；禁止手工编辑生成物正文（CR-DATA-06）。
- 重新生成前保持 grpcio-tools 1.62.3 与 protobuf 4.25.x 版本族，否则脚本拒绝执行。

## 与 system 侧的边界

| 项 | system（Go，已落地） | cognitive（Python，本组件） |
|---|---|---|
| 迁移目录 | `services/system-core/migrations/` | `services/agent-runtime/migrations/`（T04 落地） |
| 连接变量 | `DATABASE_URL` | `DATABASE_URL_COGNITIVE` |
| 回放入口 | `go run ./cmd/api -migrate` | Python 迁移入口（T04 落地） |

- 禁止两侧共用写账号；禁止 Go runner 发现 Python 迁移；禁止根级公共迁移池（CR-DATA-01/02/03）。
- 认知侧独占写 cognitive schema，不读写 system schema；产物正文存文件 + `storage_key` 地址制（D5），数据库与回执不承载正文。
## 最小离线 eval（T24，D8）

```bash
.venv/bin/python eval/run.py --dataset eval/datasets/simple_qa.csv
```

- 数据集 CSV 必须包含 `question` 与 `gold_answer` 两列，缺失字段启动即报错（R9）。
- 检索链复用生产装配（T16 双腿 + T17 图组件，无第二套实现）；judge 走 `LlmJson` 结构化评分（CR-REG-13）。
- 仅输出已执行的量化结果（逐条得分与平均分），不宣称未执行的评测结论（CR-DISC-07）。
- 需要真实 Postgres（`DATABASE_URL_COGNITIVE`）与 LLM 端点环境；属 T25 统一验证范围的离线工具，不进主链路。