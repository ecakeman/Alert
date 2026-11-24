> Source: migrated from AlertFlow stage-1 evidence (baseline e6aeb3d); conclusions unchanged.
# U01 整改基线当前状态实测

- 记录日期：2026-11-23
- 代码基线：`e6aeb3d7eaaafe6efdfc46b186efd53402267647`（分支 `rework/e6aeb3d-baseline`）
- 测试总原则：本项目为简历展示级项目，测试只做必要最小集；全量测试 / E2E / 真实模型 / 真实 K8s 默认不跑。

## 1. 目标

在整改起点记录 Go / Python / 前端 / Compose 的真实可执行状态，作为后续判断"是否回归"的对照。本记录不修复任何失败，只如实记录。

## 2. 环境

| 项 | 值 |
|---|---|
| 执行环境 | WSL Ubuntu-24.04（`/home/sancho/workspace/projects/AlertFlow`） |
| git | 2.43.0 |
| Go | 未安装（`go` 不在 PATH） |
| Python | 3.12.3 |
| Node / npm | v22.23.1 / 10.9.8 |
| Docker / Compose | 29.6.1 / v5.2.0 |

## 3. 命令矩阵

| 组件 | 命令 | 工作目录 | 耗时 | 结果 | 首因 |
|---|---|---|---|---|---|
| Go | `go build ./...`、`go vet ./...`、`go test ./internal/system/... ./internal/monitoring/...` | `services/system-core` | — | 未执行 | WSL 内未安装 Go，无法编译/测试 |
| Python | `python3 -m compileall -q src` | `services/agent-runtime` | <1s | 通过 | — |
| Python | `pytest tests/test_runtime.py -q` | `services/agent-runtime` | — | 未执行 | 测试总原则不跑 + 依赖未安装 |
| 前端 | `npm run build`（含 `vue-tsc --noEmit`） | `apps/console` | — | 未执行 | `node_modules` 不存在，未执行 `npm install` |
| Compose | `docker compose --env-file .env.example -f compose.yaml config -q` | `deploy/compose` | <1s | 通过 | — |

## 4. 结果说明

- **Python 语法级**：`compileall` 退出码 0；生成的 `__pycache__` 已被 `.gitignore` 忽略，工作树保持干净。
- **Compose**：配置可正常解析（`config -q` 退出码 0），未启动任何容器、未下载镜像。
- **前端**：`package.json` 脚本为 `dev` / `build` / `preview`，无独立 `typecheck` 脚本；因无 `node_modules`，build 未执行。

## 5. 未执行项

1. Go 侧全部（build / vet / 定向测试）：WSL 内未安装 Go。
2. Python 定向测试：按测试总原则不跑，且依赖未安装。
3. 前端 build：`node_modules` 缺失，且未执行 `npm install`（避免锁文件变更）。

## 6. 最小可复现基线

1. `cd /home/sancho/workspace/projects/AlertFlow && git switch rework/e6aeb3d-baseline`。
2. `cd services/agent-runtime && python3 -m compileall -q src`，期望退出码 0。
3. `cd deploy/compose && docker compose --env-file .env.example -f compose.yaml config -q`，期望退出码 0。
4. `apps/console` 在安装依赖前不具备可复现构建条件，记录为未执行。
5. Go 侧待补装 Go 工具链后再复测。

## 7. 限制与风险

- Go 工具链缺失是当前最大阻塞；阶段 1 的 `internal/domain` 骨架需要 Go 编译验证，进入阶段 1 前需补装。
- 前端构建未验证，后续改 API Base URL 时需在具备依赖的环境补测。
- 未连接任何真实外部服务（PG / Kafka / Redis / 模型 / RAG），以上均为静态或配置级结论。