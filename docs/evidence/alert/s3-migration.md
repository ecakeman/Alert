# S3 一次性目录搬迁 — 证据（T-S3.1 + T-S3.2）

- 日期：2026-11-24
- 基线：S2 末 `326f320`（evidence）/ `8053c5c`（S2.8 代码）
- 原则：零降级；一 task 一 commit；S3 是唯一允许"移动 + import 同 commit"的例外。
- 规格文档：`.codeartsdoer/specs/alert_struct_rework/{design,tasks}.md`

## 1. task 落地

| task | commit | 内容 |
|---|---|---|
| T-S3.1 | `d191f5d` | `git mv go.mod/go.sum/cmd/internal/migrations` → `services/system-core/`（全部 rename 识别）；module path `github.com/ecakeman/alert` → `github.com/ecakeman/alert/services/system-core`；全部 `.go` import 前缀同步替换；`migrations/embed.go` 由 `//go:embed *` 收紧为 `//go:embed *.sql`（Discover 已按 `.sql` 后缀过滤，行为等价）；无 go.work/replace/第二份 internal；无业务改动 |
| T-S3.2 | （本 commit） | 搬迁后回归验证与证据写入 |

## 2. 验证记录（工作目录 `services/system-core`，三态）

| 检查 | 命令 | 结果 |
|---|---|---|
| 构建 | `go build ./...` | ✅ 通过（Go 1.27.4，WSL go-dist） |
| 静态检查 | `go vet ./...` | ✅ 通过 |
| 根目录残留 | `test ! -e go.mod && test ! -d cmd/internal/migrations` | ✅ ROOT-CLEAN-OK |
| 旧前缀清零 | `grep -rn 'ecakeman/alert/internal\|ecakeman/alert/migrations' services/system-core` | ✅ 0 处 |
| go.work / replace | `ls go.work`；`grep replace go.mod` | ✅ 均无 |
| 迁移回放首轮 | `go run ./cmd/api -migrate`（专用空库 `s3_replay_20261124`，PG 16 compose 容器） | ✅ `applied=3 skipped=0 conflicts=0` |
| 迁移回放次轮 | 同上第二次执行 | ✅ no-op：`applied=0 skipped=3 conflicts=0` |
| SQL 字节核对 | `sha256sum migrations/*.sql` | ✅ 三个文件 SHA-256 与 S0 基线逐一一致（见下表） |
| 历史可追溯 | `git log --follow` × 3 | ✅ 均追溯到搬迁前提交（见下表） |

### 迁移 SHA-256 对照（S0 基线 → S3 实测）

| 文件 | 结果 |
|---|---|
| `0001_system_core_entities.sql` | ✅ `79c451171f13…` 一致 |
| `0002_input_events_points.sql` | ✅ `9c721b45c514…` 一致 |
| `0003_tasks_outbox_artifacts.sql` | ✅ `3bc5f8e9c4b9…` 一致 |

### git log --follow 抽查

| 文件（新路径） | 追溯链（截断） |
|---|---|
| `internal/bootstrap/usecases.go` | `d191f5d`（搬迁）→ `68896e6`（S2.5） |
| `migrations/0001_system_core_entities.sql` | `d191f5d`（搬迁）→ `671a7c7`（B1 建表） |
| `internal/contracts/event.go` | `d191f5d`（搬迁）→ `9048eb9`（S1 信封唯一化） |

## 3. 过程备注

- 搬迁前清理 2 个 S2 时代未跟踪残留文件（git 历史已删除但磁盘残留，与 HEAD 对齐后才开工）：`cmd/system-core/main.go`、`internal/bootstrap/app.go`。
- 迁移回放环境变量取自 compose 真实值：`DATABASE_URL=postgres://alert:alert@…`、`KAFKA_BROKERS=127.0.0.1:9092`（`.env.example` 中 CHANGE_ME 为占位符）。缺 `KAFKA_BROKERS` 时 `-migrate` 在 ValidateRuntime 拦截（非零退出），与既有失败路径语义一致。
- 专用回放库 `s3_replay_20261124` 与开发库 `alert` 隔离；本次仅新建，未 DROP 任何库。

## 4. S3 出口门（0.6 节）结论

新路径 `go build/vet` ✅、两次 `-migrate`（首轮 3 applied + 次轮 no-op）✅、旧前缀为空 ✅、`git log --follow` 抽查 3 文件 ✅ —— **S3 全绿，放行 S4**。