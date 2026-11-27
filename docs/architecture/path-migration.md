# 路径迁移（历史）

MS-P7 结构搬迁已完成，仓库为最终态；下表为“旧单体路径 → 最终态路径”的对照，仅作追溯，**不再并存**。

| 旧路径 | 最终态路径 |
|---|---|
| `services/contracts/` | `go/contracts/` |
| `services/platform/` | `go/platform/` |
| `services/console-bff/` | `go/services/console-bff/` |
| `services/service-registry/` | `go/services/service-registry/` |
| `services/monitor-svc/` | `go/services/monitor-svc/` |
| `services/episode-svc/` | `go/services/episode-svc/` |
| `services/task-orchestrator/` | `go/services/task-orchestrator/` |
| （单体控制面，已删除） | `go/services/*` 四域 + console-bff |

## 说明
- 搬迁窗口（MS-P4.4A~J）只搬结构、不改语义；窗口内每步可构建、可回退。
- `go.work` 在根，指向 `./go/*`；各 module 路径不变。
- 镜像构建上下文收敛到 `go/`；compose 的 `context: ../../go`。
- 校验：`bash scripts/verify-p7.sh --structure-only`（A1/A2/A3/A17/A5/A6/A7/A13）。
