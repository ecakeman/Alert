# 构建

仓库为 Go 多 module workspace（`go.work` 在根），**禁止**在仓库根执行 `go build ./...`（根不是 module，会报 `directory prefix . does not contain modules`）。

## Go（逐 module，GOWORK=off）

```bash
for m in go/contracts go/platform \
         go/services/console-bff go/services/service-registry go/services/monitor-svc \
         go/services/episode-svc go/services/task-orchestrator; do
  ( cd "$m" && GOWORK=off go build ./... )
done
```

- 每个 module 自带 `go.sum`；`GOWORK=off` 保证逐 module 独立可构建。
- 镜像：`docker compose -f deploy/compose/compose.yaml build`（build context 为 `../../go`，Dockerfile 复制 `platform`）。

## Python（agent-runtime）

```bash
docker compose -f deploy/compose/compose.yaml build agent-runtime   # 离线 wheel 优先，PIP_INDEX_URL 可覆盖
```

## 门禁脚本

```bash
bash scripts/verify-p7.sh --structure-only        # 结构/路径闸门
bash scripts/verify-p6.sh --only=direct|bridge|docs
bash deploy/split/verify-isolation.sh             # 域账号隔离
```