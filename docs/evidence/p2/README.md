# P2 阶段证据

## 代码基线

| 项 | 值 |
|---|---|
| P2 提交 | P2.1~P2.8（registry 骨架/迁移/搬迁/API/用例/事件/消费/单体适配） |
| 拓扑 | 控制台 → BFF（8080）→ registry（8081）；单体 episode/task 仍在 8082（P2 过渡） |
| 数据 | services / targets / source_states 已归 `registry` schema |

## 命令与结果

| # | 检查项 | 命令 | 结果 |
|---|---|---|---|
| 1 | 骨架与探针 | `/healthz` 200；未迁移 `/readyz` 503 | PASS |
| 2 | registry 迁移 | `-migrate` 两次 | `applied=1` → `skipped=1` |
| 3 | 存量搬迁 | `migrate-data.sh --stage P2 --confirm` | 行数对账一致；system.services 消失；重复执行为 no-op |
| 4 | 域 API 11 端点 | `scripts/p2-probe.py` | PASS（201/200/204 + 404/409/422 + 软删 404） |
| 5 | 用例/事务 | 同上（创建/目标/软删） | PASS |
| 6 | 事件发布 | 创建/改目标/软删 | outbox 各 +2（state + catalog）；GET +0；Kafka 收到 catalog |
| 7 | monitor 事件消费 | 同一事件投递 3 次 + 未知类型 | 状态回写正确；幂等 1 行；未知类型不 MarkDone |
| 8 | 单体远程适配 | worker `REMOTE_REGISTRY=true` | registry targets checked=5 / abnormal=4（探测回写成功） |
| 9 | 账号隔离 | `verify-isolation.sh` | PASS 8/8 |
| 10 | P2 门槛 | `verify-p2.sh` | 见本次运行结果 |
| 11 | 回退演练 | `rollback.sh --to P0 --confirm` → `migrate-data.sh --stage P2 --confirm` | 见演练记录 |

## 范围说明

- 路由 1（dashboard）与 11~18 依赖 BFF 跨域聚合与 episode/task 域迁移，按 tasks 规划属于 P3/P6；P2 门槛只验证 registry 直通路由（2/4/7）的两次可复现性。
- `catalog.changed` 的 targets 字段为 11 个（与 design 2.2.2-C 契约一致）；tasks 中“12 个”为笔误。
- 单体 API 的 service 写路径在 P2 不再使用（BFF 路由改由 registry 承担）；单体仅保留 episode/task/worker 的远程读写。

## 复跑方式

```bash
docker compose -f deploy/compose/compose.yaml up -d postgres kafka console-bff
bash scripts/verify-p2.sh
```

## P2 回退

`rollback.sh --to P0 --confirm`：registry 三表移回 system、重建 9 条跨域 FK、清 registry 版本行、状态文件置 `P0/rolled-back`；
恢复用 `migrate-data.sh --stage P2 --confirm`（状态文件已非 complete，可再次搬迁）。