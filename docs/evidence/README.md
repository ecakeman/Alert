# docs/evidence 索引

> 索引原则：历史证据正文与结论零改写（本索引只追加归属与迁移影响说明）；**历史证据不代表当前 Alert 仓库的通过状态**。各文件结论以文件自身为准；冒烟级运行验证以 S5 回归为唯一当前口径。
> **MS-P7 结构搬迁影响（2026-11-26）**：本目录 `p0`~`p3` 证据正文冻结未改；文中出现的 `services/` 旧路径请按《路径变更对照表》（`docs/architecture/path-migration.md`）回查。当前结构为 `go/` + `python/`。

## 一、历史证据（`history/alertflow-stage1/`，7 份）

> 来源：AlertFlow 阶段一证据迁移（基线 `e6aeb3d`），各文件头部已注明 "Source: migrated..."。记录日期 2026-11-05 ~ 2026-11-23。

| 文件 | 被测对象 / 基线 | 结论（历史态） | 迁移影响说明 |
|---|---|---|---|
| `integration-e2e.md` | AlertFlow `advence`（`e175b63`）+ rag `long`（`450e454`），2026-11-05 | 集成侧版本锁定快照，非当前部署配置 | 原文零改动；文中 Compose/镜像信息为历史快照 |
| `stage1-summary.md` | 分支 `rework/e6aeb3d-baseline` | 阶段一 task 完成报告（历史） | 原文零改动 |
| `u00-baseline.md` | AlertFlow 仓库 `e6aeb3d` | 整改基线事实与保护声明 | 原文零改动；文中绝对路径为历史环境快照 |
| `u01-status.md` | AlertFlow `e6aeb3d` | Go/Python/前端/Compose 真实状态实测 | 原文零改动 |
| `u02-patch-triage.md` | AlertFlow `e6aeb3d..5ad6c3c` | 10 个上游提交取舍结论 | 原文零改动 |
| `u03-domain.md` | AlertFlow `e6aeb3d` | 业务不变量固定（纯规则） | 原文零改动 |
| `u05-skeleton.md` | 本仓库 Alert（T200 系列，2026-11-23） | Go 骨架与前端移植验收 | 原文零改动；文中模块路径为搬迁前形态 |

## 二、当前证据 · 业务块 B0~B6（`alert/`，2026-11-23）

> 被测环境：真实 PG16 + Kafka KRaft；原则零降级。结论均为"通过（历史执行态）"，不代表当前工作区状态。

| 文件 | 被测内容 | 结论 | 迁移影响说明 |
|---|---|---|---|
| `u06-business-b0.md` | B0 真实运行基座 | 通过 | S3 搬迁后文中路径为旧形态，仅位置参考 |
| `u06-business-b1.md` | B1 数据层与迁移（14 表） | 通过 | 同上 |
| `u06-business-b2.md` | B2 persistence 仓储 | 通过 | 同上 |
| `u06-business-b3.md` | B3 application 用例 | 通过 | 同上 |
| `u07-b4.md` | B4 路由 1~15 | 通过 | 同上 |
| `u08-b5.md` | B5 Outbox + Kafka | 通过 | 同上 |
| `u09-b6.md` | B6 探测引擎 | 通过 | 同上 |

## 三、当前证据 · 本次结构收敛 S0~S3（`alert/`，2026-11-24）

| 文件 | 被测内容 | 结论 | 迁移影响说明 |
|---|---|---|---|
| `s0-baseline.md` | S0 基线冻结（T-S0.1，基线 `5c6dc86`） | 快照完成（不代表通过状态） | 声明即"整改前快照" |
| `s1-summary.md` | S1 语义收敛 16 task（`5c6dc86` → `a9095ca`） | 全绿 | 原文零改动 |
| `s2-summary.md` | S2 结构拆分 8 task（`6d16197` → `8053c5c`） | 全绿 | 含 7 条如实披露 |
| `s3-migration.md` | S3 目录搬迁（`d191f5d` + 证据 `ac94e73`） | 全绿，放行 S4 | 原文零改动 |

## 四、清单差异注明（以实际为准）

1. 历史组规格记"六份"，实际 **7 份**（integration-e2e 单独成文），以上表为准。
2. `alert/` 目录 tasks.md 验收期望 9 份（7 移入 + s0 + s3），实际 **11 份**：S1/S2 执行期间各自产出了 `s1-summary.md`、`s2-summary.md` 阶段证据，随本索引一并归档。
3. S5 完成后将新增 `alert/s5-regression.md`（design 2.4.14 预留），届时仅追加本索引一行。

## 五、归档纪律

- 本次归档为纯 `git mv`（14 份正文零改动，`git diff --stat` 全部 rename）+ 本索引新建。
- `git log --follow` 对全部移动文件可追溯（抽查含 `u01-status.md`、`u07-b4.md`）。
- 后续新证据只进 `alert/`；`history/` 永不增删改。