# deploy/split

微服务拆分的搬迁与回退脚本。P0 只提供只读预检与回退骨架，真正的移表动作在 P2~P5 各阶段任务内补齐。

## S0 预检

```bash
bash deploy/split/migrate-data.sh --dry-run --stage P0
```

- 只读：记录 13 张表行数与 `pg_total_relation_size`、迁移版本、未发布 outbox 数、近 5 分钟 outbox 写入数；
- 输出快照：`deploy/split/snapshots/<UTC 时间戳>.json`；
- 写状态文件：`deploy/split/.split-state.json`（临时文件 + `mv` 原子替换），字段为 `stage/phase/started_at/snapshot`；
- 任何脚本重启都先读该状态文件，再决定继续或回退；不允许"半搬迁"状态。

## 回退骨架

```bash
bash deploy/split/rollback.sh --to P0 --dry-run
```

- `P0`：R1（停新服务，P0 无新服务为 no-op）+ R5（恢复 system-core api/worker，由操作者启动）；
- `P2~P5`：R2~R4 尚未实装，脚本显式返回非零（退出码 3），禁止假回退；
- 危险动作必须同时提供 `--confirm`，否则退出码 4。

## 状态文件

`.split-state.json` 为运行时文件，不应提交；示例见 `.split-state.example.json`。