# 路径变更对照表（MS-P7）

> 记录 2026-11-26 双语言结构搬迁（MS-P4.4A~J）的旧→新路径映射。`docs/evidence/p0`~`p3` 等历史证据保持冻结，按原路径解读时以本表回查。

## 1. 不变项

| 项 | 说明 |
|---|---|
| Go module 路径 | 仍为 `github.com/ecakeman/alert/services/<name>`，import 语句零改动 |
| 容器名 / 端口 / 服务名 | 不变 |
| 语言无关目录 | `apps/`、`deploy/`、`contracts/`、`docs/`、`scripts/` 位置不变 |
| `go.work` | 仍在仓库根，7 条 `use` 改指 `./go/` |

## 2. 目录映射

| 旧路径 | 新路径 |
|---|---|
| `services/contracts/` | `go/contracts/` |
| `services/platform/` | `go/platform/` |
| `services/system-core/` | `go/services/system-core/` |
| `services/console-bff/` | `go/services/console-bff/` |
| `services/service-registry/` | `go/services/service-registry/` |
| `services/monitor-svc/` | `go/services/monitor-svc/` |
| `services/episode-svc/` | `go/services/episode-svc/` |
| `services/agent-runtime/` | `python/agent-runtime/` |

## 3. 构建与编排

| 项 | 旧 | 新 |
|---|---|---|
| `go.mod` replace | `../platform`、`../contracts` | `../../platform`、`../../contracts` |
| compose `build.context` | `../..` | `../../go` |
| Dockerfile platform | `COPY services/platform services/platform` | `COPY platform platform` |
| Dockerfile 服务 | `COPY services/<svc> services/<svc>` | 不变（相对 `go/` 仍是 `services/<svc>`） |
| Go 构建入口 | 根目录 `go build ./...`（本就不可用） | 逐 module：`go -C go/services/<m> build ./...`，或 `GOWORK=off` |

## 4. 脚本与门禁

| 项 | 变化 |
|---|---|
| `scripts/verify-p0.sh` / `verify-p3.sh` | 包路径前缀加 `go/` |
| `scripts/verify-module-boundary.sh` | `list -deps` 包模式改 `./go/services/<m>/...`；import 正则不变 |
| `deploy/split/migrate-data.sh` / `rollback.sh` | `$ROOT/services/` → `$ROOT/go/services/` |
| `scripts/verify-p7.sh` | 新增：结构 / 构建 / 路径 / 边界 / 编排 / 证据 / Python 七段门禁 |

## 5. 冻结与豁免

- 冻结：`docs/evidence/p0`~`p3` 正文零改动（A4）。
- 豁免：`apps/console/node_modules/` 与 `python/agent-runtime/.venv/` 内的符号链接不计入 A17；REST 路由 `/api/v1/services/` 不计入 A7/A8。
- 历史旧路径仅在本表与冻结证据中出现。

## 6. 回退

| tag | 指向 | 用途 |
|---|---|---|
| `ms-p7-p44` | P4.4 完成态 | 彻底回到搬迁前 |
| `ms-p7-baseline` | S0 之后 | 默认回退点（含 go.sum 修复） |
| `ms-p7-done` | S9 之后 | 解冻后新基线 |

```bash
git reset --hard ms-p7-baseline                    # 本地未推送
git revert --no-commit ms-p7-baseline..HEAD         # 已推送或需保留历史
```

## 7. 门禁

```bash
bash scripts/verify-p7.sh                    # 全量
bash scripts/verify-p7.sh --structure-only   # 仅结构 / 路径
bash scripts/verify-p7.sh --no-docker        # 跳过编排运行时
```
