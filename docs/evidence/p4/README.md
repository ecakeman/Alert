# P4 阶段验收证据（MS-P4.10）

## 1. 结论

`bash scripts/verify-p4.sh` 退出 0：**PASS=26 / FAIL=0 / SKIP=0**。

覆盖项：P7 结构门禁、7 module `GOWORK=off` 构建、5 服务探活、立案复用、输入去重、dispatchable 字段、解除判定、pending-dispatch、BFF 路由 11/12/13、compose.p4 编排与端口约束。

## 2. 事件载荷样例（episode.dispatchable.v1）

```json
{"reason": "EPISODE_RESOLVED", "episode_id": "4ec06fdc-baec-4d31-afd2-820f3e2e3111", "service_id": "73bf858f-b9a4-4b5b-9dd1-1440c58d4c03", "episode_status": "RESOLVED", "input_version_to": 1, "monitor_watermark": 0, "artifact_readiness": {"hashes_match": false, "investigation": false, "diagnosis_report": false}, "input_version_from": 1, "app_event_watermark": 4611686018427387904, "final_input_version": 1, "processed_input_version": 1}
```

## 3. 阶段交付

| task | 提交 |
|---|---|
| P4.4A~J 结构搬迁 | 10 个提交 + tag `ms-p7-done` |
| P4.5 dispatchable 快照 | `1757eb8` |
| P4.6 11 条域端点 | `8cd3a48` |
| P4.7 消费者 + 发布器 | `9b79e21` |
| P4.8 task 侧零直读 + 对账 | `1aeb753` |
| P4.9 BFF 切 episode + 容器化 | `9d72c39` |
| P4.10 门禁 | 本提交 |

## 4. 已知遗留（均有后续 task 承接）

| 项 | 说明 | 承接 |
|---|---|---|
| BFF 列表/详情跨域字段 | episode 域端点只返回域内事实，`service_name`/活动任务等由 BFF 聚合补齐 | P6.1 |
| dashboard 500 | 单体 dashboard 的 trend 仍查已搬迁的 monitor 表 | P6.1 |
| P4→P3 运行态回退 | `rollback.sh --to P3` 当前输出“R2~R4 未实装”；代码回退为 `git revert P4.5..P4.9`，运行态为 `docker rm -f alert-episode-svc` + BFF 回 compose.p3 | P4.10 后续补实装（已在 P5 前标注） |

## 5. 回退说明

- 代码：`git revert 1757eb8..9d72c39`（保留搬迁窗口）
- 运行态：停 `alert-episode-svc`，BFF 回 `compose.p3.yaml`
- 数据：episode schema 与数据保留，不删不迁

## 6. 门禁输出

```
见终端输出：PASS=26 FAIL=0 SKIP=0（structure/build/alive/usecase/routes/compose）
```
