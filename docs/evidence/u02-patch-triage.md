> Source: migrated from AlertFlow stage-1 evidence (baseline e6aeb3d); conclusions unchanged.
# U02 阶段一补丁取舍（e6aeb3d..5ad6c3c）

- 记录日期：2026-11-23
- 范围：上游阶段一 10 个提交
- 原则：保留正确的不变量与测试语义，不迷信补丁的当前代码形态；冲突时按目标边界重写，不整体合并 `5ad6c3c`。

## 1. 逐提交取舍

| # | 提交 | 主题 | 结论 | 本轮处置 | 映射单元 | 风险 / 验收 |
|---|---|---|---|---|---|---|
| 1 | `7ca4152` | Go 回调按 live Lease fencing | 保留语义 | 作为 Fencing 不变量重写，不复用旧 `task.go` 写法 | U07 → T25/T26 | 旧代次晚到必须拒绝；用例覆盖 |
| 2 | `8aa1caa` | Python 拒绝旧执行 / 心跳失败取消 / 产物 fencing | 保留语义 | 保留 receipt fencing 概念，审查取消传播后再落 U13 | U07/U13 → T25/T303/T304 | 失权即取消；旧代次无副作用 |
| 3 | `eac56bf` | APP 触发冻结 monitor snapshot/watermark | 保留 | 纳入 Timeline 输入边界不变量 | U03/U06 → T23 | 诊断只读绑定范围 |
| 4 | `8246b4f` | FAILED 终态不被自动 backfill 重开 | 保留 | 终态规则 + 人工重试用例 | U03/U07/U12 → T29 | 终态状态与 generation 不变 |
| 5 | `9493d41` | Monitor target 变更重算聚合状态 | 保留 | 纳入 Monitor 聚合规则 | U11 → T216 | 目标变更后聚合正确 |
| 6 | `045e7e1` | Workflow 限制工具边界 | 部分保留 | 保留 allowlist 设计目的；`read_only` 机制因元数据丢失不完整，改用 MCP SDK 重写 | U19 → T503 | 可信策略层校验，不依赖 prompt |
| 7 | `5a01b9a` | 证据来源溯源 | 保留语义 | 改为明确 Evidence schema，补引用完整性校验 | U15 → T308 | 来源类型可区分、可追溯 |
| 8 | `e37fbde` | 对齐阶段一测试与运行时不变量 | 保留测试价值 | 仅继承"验证业务行为"的语义，不继承绑定旧内部结构的测试 | — | 不写只绑定当前实现结构的测试 |
| 9 | `69260e5` | Java RevisionCache 过滤列表绕过缓存 | 不迁移当前实现 | Java 缓存模块将移除；Go Query 默认不启用缓存，有性能证据再做 Cache-Aside | —（Java 移除） | 不保留 Java RevisionCache |
| 10 | `5ad6c3c` | 合并阶段一改进 | 不整体合并 | 仅作本地只读参考 tag；按独立语义逐项重写 | 参考 | 不作为新基线 |

## 2. 按单元汇总（各 U 需继承的语义）

- **U03（业务不变量）**：继承 `eac56bf`（输入快照/水位线边界）、`8246b4f`（终态）、`7ca4152`（Lease 语义，用于 INV-08）。
- **U06（服务级调度）**：继承 `eac56bf` 的输入边界；服务级串行为新增设计。
- **U07（Fencing Guard）**：继承 `7ca4152`、`8aa1caa`、`8246b4f`。
- **U11（Monitor 聚合）**：继承 `9493d41`。
- **U12（Python 恢复）**：继承 `8246b4f` 的终态语义（终态/旧代次不恢复）。
- **U13（Python 取消/关闭）**：继承 `8aa1caa` 的失权取消与产物 fencing。
- **U15（产物/证据）**：继承 `5a01b9a` 的 provenance 思路，改为 schema。
- **U19（工具策略）**：继承 `045e7e1` 的 allowlist 目的，重写实现。
- **Java 侧**：不继承任何运行实现。

## 3. 明确排除清单

1. **Java RevisionCache 实现**（`69260e5`）：Java 运行面移除，缓存默认不启用。
2. **绑定旧内部结构的测试**（`e37fbde` 中的部分）：只保留验证业务行为的用例语义。
3. **merge commit 整体合并**（`5ad6c3c`）：仅只读参考，不 cherry-pick 区间、不整体 merge。

## 4. 门禁 0 判定

| 门禁项 | 判定 | 证据 |
|---|---|---|
| 整改分支起点 == `e6aeb3d` | 是 | `u00-baseline.md` §1 |
| main / origin-main 仍为 `5ad6c3c`，未 reset/强推 | 是 | `u00-baseline.md` §3 |
| 10 个提交逐项有处置（无“待定”） | 是 | 本文档 §1（10/10） |
| 当前状态实测覆盖 Go/Python/前端/Compose，未执行有原因 | 是 | `u01-status.md` §3–§5 |
| 参考 tag 存在且不参与构建 | 是 | `u00-baseline.md` §4 |

**门禁 0 结论：通过**，可进入阶段 1（业务不变量固定）。存在一处执行环境缺口：WSL 未安装 Go，阶段 1 编码前需补装（不影响阶段 0 判定）。