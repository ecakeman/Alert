> Source: migrated from AlertFlow stage-1 evidence (baseline e6aeb3d); conclusions unchanged.
# 阶段一完成报告（基线校准 + 业务不变量固定）

- 报告日期：2026-11-23
- 分支：`rework/e6aeb3d-baseline`（起点 `e6aeb3d`）
- 原则：展示级项目，测试只做必要最小集；全量/E2E/真实模型默认不跑。

## 1. task 勾选状态

| 会话包 | task | 状态 |
|---|---|---|
| A 干净基线 | T01–T05 | 完成 |
| B 参考+实测+证据 | T06–T11 | 完成（Go 实测=未执行：安装前；Python 已补测） |
| C 补丁取舍+门禁 0 | T12–T14 | 完成（门禁 0 通过） |
| D 骨架+状态机 | T15–T17 | 完成 |
| E INV-01~05 | T18–T22 | 完成 |
| F INV-06~10 | T23–T27 | 完成 |
| G INV-11~12 + U03 证据 | T28–T30 | 完成 |
| H U04 集成 + 门禁 1 | T31–T33 | T31/T32 未执行（测试总原则豁免）；T33 完成 |

## 2. 门禁 0（基线）

| 项 | 判定 |
|---|---|
| 分支起点 == `e6aeb3d` | 是 |
| main / origin-main 仍为 `5ad6c3c` | 是 |
| 10 个补丁逐项取舍 | 是（`u02-patch-triage.md`） |
| 当前状态实测含未执行原因 | 是（`u01-status.md`） |
| 参考 tag 就绪且不参与构建 | 是 |

**结论：通过。**

## 3. 门禁 1（不变量）

| 项 | 判定 |
|---|---|
| INV-01~12 各有直接测试或注明后续覆盖 | 是（`u03-domain.md`） |
| `go test ./internal/domain/...` 无外部依赖通过 | 是（14 用例 PASS） |
| 非法状态转换（含 ACTIVE→ARCHIVED、终态重开）全部拒绝 | 是 |
| Fencing 拒绝码稳定、非 INTERNAL | 是 |
| `internal/domain` 无 pgx/http/gin/grpc/internal/system 依赖 | 是（CLEAN） |
| 3 个 DB 集成用例 | 未执行（按“测试只做必要最小集”豁免） |

**结论：通过（DB 集成用例经用户测试策略豁免）。**

## 4. 已交付物

- 规则包 `services/system-core/internal/domain/`：状态机、来源状态、输入指纹、APP 事件幂等、输入边界、服务级执行权、fencing、结果隔离、归档与产物校验、终态/人工重试。
- 证据：`u00-baseline.md`、`u01-status.md`、`u02-patch-triage.md`、`u03-domain.md`。
- 本地只读 tag：`ref/upstream-stage1-5ad6c3c`。

## 5. 遗留与风险

- Go 工具链已补装于 `~/.local/go`（1.27.4），未写入 shell 配置；后续命令需 `export PATH=$HOME/.local/go/bin:$PATH`。
- DB 集成、真实并发、真实 Kafka/模型/RAG 全部未执行，按展示级原则在最终验收披露。
- `internal/domain` 目前为新增独立包，尚未接入 `internal/system`（阶段 2 的 U05 起逐步接入）。

## 6. 下一步建议

进入阶段 2 首 task **T201**：`app` / `store` 目录骨架 + `system.Store` 改为 facade 委托（行为零变化）。推进前需按 tasks.md 规则先细化 T201 字段并由用户确认。