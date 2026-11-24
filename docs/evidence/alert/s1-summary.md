# S1 语义收敛 — 阶段证据

- 日期：2026-11-24
- 基线：`5c6dc86`（S0 冻结）→ 收敛后 HEAD：`a9095ca`
- 原则：零降级；一 task 一 commit；白名单外改动均如实披露进 commit message。

## 1. 16 个 task 全部完成

| task | commit | 内容 |
|---|---|---|
| S0.1 | `ec6ffda` | 基线冻结（HEAD/目录快照/build/B7 保留清单/路由快照） |
| S1.1 | `48d4d73` | 事件信封契约源（JSON Schema v1 + README，语言无关） |
| S1.2 | `9048eb9` | Go 事件信封唯一化（`contracts.Event` + `OutboxRecord`，12 文件签名统一） |
| S1.3 | `aa4957b` | 消费端未知版本失败语义（不静默 MarkDone） |
| S1.4 | `c36d7e7` | monitor→EpisodeDriver 依赖倒置 |
| S1.5 | `257266c` | domain MONITOR 三模式真值表（纯新增） |
| S1.6a/b | `abe3ef4`+`8eebceb` | bootstrap 暴露 ServiceRepo（拆分提交） |
| S1.6b | `ff949c3` | 两处 MONITOR 重算收敛到 domain + 模式三值校验 |
| S1.7 | `5dedbb5` | 行模型下沉（主链四表 + dashboard 适配 + 集成测试修复） |
| S1.8 | `051238c` | 行模型下沉（其余 + B7 表 + 过渡别名清理） |
| S1.9 | `908dcec` | 端口类型精简（10 个存储-only 字段删除，INSERT 时间列 DB 默认） |
| S1.10 | `5ce68ea` | 看板真实读（recent/trend，删 Counts/Trend 与固定空数组） |
| S1.11 | `f355a63` | status SQL 真过滤 + total 口径一致 |
| S1.12 | `43b751e` | monitor.DueTargets 调度编排外移 |
| S1.13 | `51ae49e` | probe.Engine 去 UOW/仓储（窄端口消费快照） |
| S1.14 | `982573f` | apperr.Normalize + httperr 收口 |
| S1.15 | `71fce7a` | episode 投影视图去 transport domain import |
| S1.16 | `a9095ca` | 消费用例迁出 bootstrap（Consumer.Handle） |

## 2. 调研问题闭环

| 级别 | 问题 | 状态 |
|---|---|---|
| P0 | 事件契约两套/调度列泄漏 | ✅ 单一 `contracts.Event`，`rg OutboxEvent` = NONE |
| P0 | monitor→episode 具体依赖 | ✅ EpisodeDriver，`rg 'application/episode' monitor` = NONE |
| P0 | DB 行结构泄漏 application | ✅ postgres/model 11 表行模型，`func scan` = NONE |
| P1 | MONITOR 双算法 | ✅ domain 单源真值表 + 一致性测试 |
| P1 | 看板假链路 | ✅ 真实 recent/trend（Counts/Trend 删除） |
| P1 | status 假筛选 | ✅ SQL EXISTS 过滤 + total 同口径 |
| P1 | probe 编排越层 | ✅ DueTargets 外移，probe 无 UOW/Tx |
| P1 | transport 依赖 domain | ✅ 生产代码清零（httperr_test 测试输入保留，披露） |
| P1 | bootstrap 消费逻辑 | ✅ Consumer.Handle 用例化 |
| P2 | 死接口/占位 | ✅ port.Prober/TaskDispatcher 前者删除、Counts/Trend 删除 |

## 3. S1 出口检查（实测）

| 检查 | 结果 |
|---|---|
| `go build ./...` / `go vet ./...` | OK |
| `go test ./internal/domain/...` | ok（14 用例不退化） |
| `rg 'application/episode' internal/application/monitor` | 空 |
| `rg 'internal/domain' internal/transport` | 仅 httperr_test.go（测试构造输入，披露） |
| `rg 'type Event struct' internal` | 仅 internal/contracts/event.go |

## 4. 已知披露

- transport 测试文件的 domain import 为构造测试输入所需（生产代码零依赖）。
- S1.7/S1.9/S1.10/S1.11 共 4 次白名单扩展（dashboard 适配、INSERT 时间列 DB 默认、sampleService/Episode 字面量、integration_test 构造），均已记录于对应 commit message。
- S1.6a 因 sed 未命中拆为两 commit（abe3ef4/8eebceb），历史如实保留。

## 5. 下一步

**S2（8 个 task）**：旧模块内结构拆分——port 按能力分包、handler 拆分、bootstrap 拆 wire*、cmd/api+cmd/worker；随后 S3 一次性 `git mv` 到多组件目录。