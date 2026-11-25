# P3 阶段证据（monitor 拆分）

## 代码基线

| 项 | 值 |
|---|---|
| 阶段 | P3，MS-P3.1 ~ MS-P3.9 |
| 服务 | 新增 `services/monitor-svc`（HTTP 8083，不映射宿主机端口由 compose 约束） |
| 数据 | `service_monitor_points` 已迁入 `monitor` schema（412 行对账一致）；新增 `monitor_target_projection` + `monitor_target_state` + `monitor_source_state` |
| 拓扑 | registry（8081）→ monitor（8083）→ 单体 worker（消费 monitor 事件） |

## 命令与结果

| # | 检查项 | 命令 | 结果 |
|---|---|---|---|
| 1 | monitor 骨架/迁移 | `-migrate` 两次 | `applied=1 skipped=0` → `applied=0 skipped=2` |
| 2 | points 搬迁 | `migrate-data.sh --stage P3 --confirm` | 412 行对账一致；`system.service_monitor_points` 消失 |
| 3 | 投影消费 | Kafka `service.catalog.changed.v1` | `monitor_target_projection` 4 行；`service.state.changed.v1` 跳过不阻塞 |
| 4 | 真实探测采样 | `PROBE_ENABLED=true` 探测不可达目标 | points 持续增长；`monitor_target_state=ABNORMAL`；source=`ABNORMAL` |
| 5 | 事件抑制 | 连续无变化采样 | outbox 恒为 target=1 / service=1（无变化不发） |
| 6 | 域 API | `latest-seq` / `points` / `points/trend` / `stats` | 4 端点 2xx；trend 24 桶含零值；latest-seq = max(point_seq) |
| 7 | 事件发布 | `alertflow.monitor.events.v1` | outbox 2 条 `published_at` 非空；Kafka 消息 event_id 与 outbox 一致 |
| 8 | 单体事件驱动 | 投递 ABNORMAL/NORMAL | `episode ACTIVE` → `RESOLVED`；单体 `PROBE_ENABLED=false` 无探测循环 |
| 9 | 输入边界 | 代码审查 + 构建 | 点位查询在事务外；失败返回 `DEPENDENCY_UNAVAILABLE`（503）且水位线不变；生产代码暂无调用入口，端到端留待 P4 输入链路 |
| 10 | P3 门槛 | `GO_BIN=/home/sancho/go-dist/go/bin/go bash scripts/verify-p3.sh` | 退出 0（见下方本次输出） |
| 11 | 隔离 | `verify-isolation.sh` | PASS 8/8 |
| 12 | P95 | `/internal/points/trend` × 20 | p95≈0.007s（阈值 5s） |

本次门槛输出（摘要）：

```text
PASS: go build
applied=0 skipped=2 conflicts=0
isolation: PASS 8/8
PASS: 异常→ACTIVE
PASS: 恢复→RESOLVED
PASS: latest-seq == max(point_seq) (0)
p95=0.007360s
PASS: compose.p3.yaml
P3 门槛通过
```

## 实现要点

1. **无跨域写**：采样事务只写 `service_monitor_points` + 自产状态 + `outbox`；target/service 状态以事件回传 registry。
2. **投影是 CQRS 读模型**：`monitor_target_projection` 只含 registry 配置与自产 `last_probe_at`，不含 `status/consecutive_*`；探测计算状态落在 `monitor_target_state` / `monitor_source_state`。
3. **状态变化抑制**：target 状态或 service 聚合状态变化才写事件（P0.16 等价）。
4. **过渡适配**：单体以 `REMOTE_REGISTRY=true` 读 registry、消费 monitor 事件；`PROBE_ENABLED=false` 关闭自身探测。

## 修复的既有缺陷（P3 施工中发现）

1. `loadOrder` 未登记 `REMOTE_REGISTRY / REGISTRY_BASE_URL / UPSTREAM_TIMEOUT`，导致 P2 远程仓储开关实际未生效；现已登记并验证。
2. system-core Kafka 消费者使用自动提交，处理失败仍提交；已改为 `CommitInterval=0` + 成功才 `CommitMessages`。
3. `TryResolve` 预检错误被吞为固定文案；现以 WARN 记录具体失败点（get episode / list sources / list active tasks）。

## 未执行项

- MS-P3.8 端到端：`AppendInputVersion` 在生产代码无 HTTP/定时调用入口（仅测试调用），本次以构建 + 代码审查验收；待 P4.9/输入链路联调时补测。
- 回退演练：`rollback.sh --to P2` 已实装并通过 `--dry-run`；破坏性实跑未在本次执行（避免影响正在验证的数据），可在维护窗口执行。

## 复跑方式

```bash
GO_BIN=/home/sancho/go-dist/go/bin/go bash scripts/verify-p3.sh
```

前置：`docker compose -f deploy/compose/compose.yaml up -d postgres kafka`；`provision` 已执行。

## P3 回退

```bash
bash deploy/split/rollback.sh --to P2 --confirm   # points 移回 system（P2 完成态无跨域 FK）
```