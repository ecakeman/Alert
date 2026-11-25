# P0 阶段证据

## 执行环境

| 项 | 值 |
|---|---|
| 代码基线 | P0.1~P0.18 已提交（`4997c4c`）；本文件随 P0.19 提交 |
| Go | WSL go-dist（Windows go 不可用于 UNC 仓库） |
| Docker | WSL 内 docker（Windows 侧 docker 无法连接 daemon） |
| 执行方式 | `GO_BIN=/home/sancho/go-dist/go/bin/go bash scripts/verify-p0.sh` |

## 命令清单与结果

| # | 检查项 | 命令 | 结果 |
|---|---|---|---|
| 1 | 全量构建 | `go build ./services/contracts/... ./services/platform/... ./services/system-core/...` | PASS |
| 2 | 静态检查 | `go vet <同上>` | PASS |
| 3 | contracts 单测 | `go test ./services/contracts/...` | PASS |
| 4 | platform 单测 | `go test ./services/platform/...` | PASS（真实 PG/Kafka 用例在依赖缺失时 SKIP） |
| 5 | module 边界 | `bash scripts/verify-module-boundary.sh` | PASS（含违规自测） |
| 6 | P0.16 采样预演 | `go test ./internal/application/monitor -run TestP0` | PASS（4 用例） |
| 7 | P0.17 解除预演 | `go test ./internal/application/episode -run TestP0` | PASS（4 用例） |
| 8 | P0.18 派发预演 | `go test ./internal/application/task -run TestP0` | PASS（4 用例） |
| 9 | provision 4 schema/4 role | `docker compose --profile setup run --rm provision` | PASS（幂等，二次执行仍退出 0） |
| 10 | 账号隔离反向验证 | `bash deploy/split/verify-isolation.sh` | PASS 8/8 |
| 11 | S0 预检 | `bash deploy/split/migrate-data.sh --dry-run --stage P0` | PASS（13 表快照 + 状态文件） |
| 12 | 迁移幂等 | `go run ./services/system-core/cmd/api -migrate` 两次 | PASS（`applied=3 skipped=0` → `applied=0 skipped=3`） |
| 13 | 18 路由快照可复现 | `seed + snapshot ×2 + snapshot-diff` | PASS（21 个响应，两次 diff 无差异） |

最终门槛汇总：**PASS=13，FAIL=0，SKIP=0**。

## 快照基线说明

- 成功路由 18 条 + 错误分支 3 条（`19_err_not_found=404`、`20_err_conflict=409`、`21_err_validation=400`）。
- `422` 属于 P1 BFF 的新 platform 错误契约；当前单体 L1 仍返回 `400`，基线按现状冻结，不在 P0 改动。
- 归一化：JSON 键排序、时间字段 → `<timestamp>`、UUID → `<uuid-N>`、随机请求 ID（32 hex）→ `<hex32>`。

## 执行中发现并修复的环境问题

1. Windows 侧 docker 无法连接 daemon：所有 docker 命令改在 WSL 内执行；
2. `docker exec` 内 psql 提示密码：postgres 容器注入 `PGPASSWORD`，provision 脚本显式导出；
3. provision 中 `psql -c` 的 `:'pw'` 变量不插值：改为在 bash 内生成安全 SQL 字面量；
4. PG 数据卷 collation 版本不匹配：`ALTER DATABASE ... REFRESH COLLATION VERSION`；
5. 迁移命令缺少 `KAFKA_BROKERS`：门槛脚本补齐；
6. seed 清理顺序漏 service 级 `episode_app_events` / points：已按依赖顺序补齐。

## 复跑方式

```bash
docker compose -f deploy/compose/compose.yaml up -d postgres kafka
docker exec alert-pg psql -U alert -c 'CREATE DATABASE alert_test'   # 若不存在
GO_BIN=/home/sancho/go-dist/go/bin/go bash scripts/verify-p0.sh
```

快照项需要单体 API 运行：

```bash
DATABASE_URL='postgres://alert:alert@127.0.0.1:5432/alert?sslmode=disable' \
  KAFKA_BROKERS=127.0.0.1:9092 /home/sancho/go-dist/go/bin/go run ./services/system-core/cmd/api
# 另一终端
bash scripts/snapshot-routes.sh && bash scripts/snapshot-routes.sh && bash scripts/snapshot-diff.sh
```

## P0 整体回退

P0 只有新增 module 与内部重构，无对外行为变化；回退 = 逆序 `git revert`：

```bash
git revert --no-edit 4997c4c   # P0.18
git revert --no-edit 3246c60   # P0.17
git revert --no-edit d53766b   # P0.16
git revert --no-edit 28f500a   # P0.15
git revert --no-edit d12a04c   # P0.14
git revert --no-edit 4d8128c   # P0.13
git revert --no-edit 1d9d5f9   # P0.12
git revert --no-edit 49b7d6f   # P0.11
git revert --no-edit 895de30   # P0.10
git revert --no-edit e79378b   # P0.9
git revert --no-edit 1cff146   # P0.8
git revert --no-edit b32fe34   # P0.7
git revert --no-edit fe2218e   # P0.6
git revert --no-edit 9ca3ed8   # P0.5
git revert --no-edit e5261f3   # P0.4
git revert --no-edit dabcd58   # P0.3
git revert --no-edit f2210ce   # P0.3 前置修正
git revert --no-edit afc7727   # P0.2
git revert --no-edit cc66d9d   # P0.1
```

新增但未跟踪的目录（`services/contracts`、`services/platform`、`deploy/split/snapshots`）在 revert 后按需手工删除。