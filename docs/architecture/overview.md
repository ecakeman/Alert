# 架构总览（overview）

> 描述当前仓库的真实结构与依赖方向。历史结论见 `docs/evidence/`，不在此重写。

## 1. 目标形态与目录树

多组件目录：唯一 Go module + Python 占位 + 前端 + 语言无关契约。

```text
Alert/
├─ go/services/     # 唯一 Go module（api + worker 双入口）
│  ├─ 域服务入口/        # 进程入口与 flag
│  ├─ internal/{domain,contracts,application,infrastructure,transport,bootstrap,config}/
│  └─ migrations/            # system schema SQL（仅 .sql）
├─ python/agent-runtime/   # Python 组件占位（B8 未实现）
├─ apps/console/             # Vue 控制台
├─ contracts/                # events / proto（预留）/ openapi
├─ deploy/compose/           # PG16 + Kafka(KRaft) 本地拓扑
└─ docs/{architecture,runbooks,evidence}/
```

## 2. 六层依赖方向（自上而下单向）

| 层 | 允许依赖 | 禁止依赖 |
|---|---|---|
| domain | 标准库 | pgx/gin/kafka/SQL/配置 |
| contracts | 标准库 | application/domain/infrastructure |
| application（port 七能力子包 + 用例） | domain、contracts、port/* | messaging/postgres 具体实现 |
| infrastructure（postgres/messaging/probe） | port/*、model、contracts | application 用例包、bootstrap |
| transport/http | application + dto + httperr | domain、SQL |
| bootstrap/config | 全部下层 | 被 application/transport 依赖 |

关键收敛成果：事件信封唯一化（`internal/contracts` ↔ `contracts/events/` 同源）、monitor 依赖倒置（EpisodeDriver 接口在 monitor 内）、行模型只在 `postgres/model`、probe 不持有 UOW/仓储。

## 3. api / worker 角色白名单

| 进程 | 启动内容 | 禁止内容 |
|---|---|---|
| 域服务 REST 入口 | HTTP 路由、探针、`-migrate` 入口 | probe/outbox/consumer 等后台循环 |
| 域服务后台循环 | DueTargets 调度、probe 引擎、outbox publisher、consumer | HTTP Server、迁移 |

装配对应 `internal/bootstrap/{api,worker,lifecycle,migrate}.go`；无万能 `New`。

## 4. 阶段顺序（S0~S5）

S0 基线冻结 → S1 语义收敛（16 task）→ S2 结构拆分（8 task）→ S3 目录搬迁（git mv + module path）→ **S4 契约/文档/证据收口（本阶段）** → S5 最终最小回归。每 task 一个本地 commit，阶段间有放行门（见 `.codeartsdoer/specs/alert_struct_rework/tasks.md` 0.6）。