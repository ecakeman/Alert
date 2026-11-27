# Runbook：构建与静态检查

工作目录：`go/services`（唯一 Go module）。

```bash
go build ./...
go vet ./...
go test ./internal/domain/... ./internal/config/ ./internal/transport/http/router/
```

说明：

- `go.mod`/`go.sum` 位于 `go/services/`；仓库根没有 `go.mod`，根目录执行 `go build` 无意义。
- 全量 `go test ./...` 中依赖真实 PG 的仓储测试需要 `TEST_DATABASE_URL`，未设置时跳过或记未执行，不视为失败。
- 前端构建（仅验收需要时）：工作目录 `apps/console`，`npm run build`。