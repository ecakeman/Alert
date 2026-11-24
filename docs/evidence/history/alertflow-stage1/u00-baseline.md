> Source: migrated from AlertFlow stage-1 evidence (baseline e6aeb3d); conclusions unchanged.
# U00 整改基线事实与保护声明

- 记录日期：2026-11-23
- 执行环境：WSL Ubuntu-24.04
- 源码工作区（`$SRC`）：`/home/sancho/workspace/projects/AlertFlow`

## 1. 仓库与远端

| 项 | 值 |
|---|---|
| 仓库 URL | `https://github.com/ecakeman/AlertFlow.git` |
| 克隆日期 | 2026-11-23 |
| `origin/main` | `5ad6c3c13cb7a1914869df1cd0ca01a6114e324f` |
| 整改基线 | `e6aeb3d7eaaafe6efdfc46b186efd53402267647` |
| 整改分支 | `rework/e6aeb3d-baseline`（HEAD = 基线 SHA） |

## 2. 维护分支指向（BASE-10）

克隆时以下远端分支均指向同一 SHA：

| 远端分支 | SHA |
|---|---|
| `origin/backend` | `5ad6c3c13cb7a1914869df1cd0ca01a6114e324f` |
| `origin/integration` | `5ad6c3c13cb7a1914869df1cd0ca01a6114e324f` |
| `origin/agent` | `5ad6c3c13cb7a1914869df1cd0ca01a6114e324f` |
| `origin/frontend` | `5ad6c3c13cb7a1914869df1cd0ca01a6114e324f` |

## 3. main 保护声明

- 未 `reset`、未强制推送、未以整改分支替换 `main`。
- 改动前 `main` = `origin/main` = `5ad6c3c13cb7a1914869df1cd0ca01a6114e324f`。
- 改动后 `main` = `origin/main` = `5ad6c3c13cb7a1914869df1cd0ca01a6114e324f`（一致）。
- 整改分支 `rework/e6aeb3d-baseline` 未推送远端。

## 4. 参考 tag 与阶段一提交索引

本地只读 tag：`ref/upstream-stage1-5ad6c3c` → `5ad6c3c13cb7a1914869df1cd0ca01a6114e324f`（不推送、不参与构建、不整体合并）。

`e6aeb3d..5ad6c3c` 共 10 个提交（按时间正序）：

| # | SHA | 标题 | 时间（+0800） |
|---|---|---|---|
| 1 | `7ca4152` | fix(system): fence task callbacks by live lease | 2026-11-20 13:43:31 |
| 2 | `8aa1caa` | fix(agent): stop stale task executions | 2026-11-20 13:44:29 |
| 3 | `eac56bf` | fix(system): freeze APP-triggered monitor snapshots | 2026-11-20 14:33:22 |
| 4 | `8246b4f` | fix(system): keep failed tasks terminal | 2026-11-20 14:41:20 |
| 5 | `9493d41` | fix(system): recompute monitor state on target changes | 2026-11-20 15:48:24 |
| 6 | `69260e5` | fix(integration): bypass cache for filtered lists | 2026-11-20 16:47:26 |
| 7 | `045e7e1` | fix(agent): enforce workflow tool boundaries | 2026-11-20 17:01:59 |
| 8 | `5a01b9a` | fix(agent): preserve evidence provenance | 2026-11-20 17:10:58 |
| 9 | `e37fbde` | test: align stage-one regressions with runtime invariants | 2026-11-20 18:46:23 |
| 10 | `5ad6c3c` | Merge reviewed Claude stage-one improvements | 2026-11-20 20:51:10 |

## 5. 未归属工作与数据卷（T05）

- `git status` 工作树干净；无 stash；无未推送提交；无本地 tag（除本记录新建的参考 tag）。
- 全新克隆，无历史未提交改动，无 AlertFlow 相关 Docker 数据卷遗留。
- 结论：**干净**，无阻塞。

## 6. 门禁 0 判定

| 门禁项 | 判定 | 证据 |
|---|---|---|
| 整改分支起点 == `e6aeb3d` | 是 | §1 |
| main / origin-main 仍为 `5ad6c3c`，未 reset/强推 | 是 | §3 |
| 10 个提交逐项有处置（无“待定”） | 是 | `u02-patch-triage.md` §1 |
| 当前状态实测覆盖 Go/Python/前端/Compose，未执行有原因 | 是 | `u01-status.md` §3–§5 |
| 参考 tag 存在且不参与构建 | 是 | §4 |

**门禁 0 结论：通过**。可进入阶段 1；阶段 1 编码前需在 WSL 补装 Go 工具链（当前唯一执行环境缺口，不影响阶段 0 判定）。