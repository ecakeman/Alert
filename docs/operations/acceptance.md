# 本轮验收记录

记录日期：2026-11-15。Kubernetes 没有在真实集群执行，下面不把它写成部署通过。国内镜像没有逐个 `docker pull` 成功，所以 `deploy/images.env.example` 只保留了原来的镜像名，没有写入未核验的镜像地址。

## 已执行

- `go test -count=1 ./internal/monitoring/...`：通过。本机没有 gcc，`go test -race` 无法启动。
- `go build -o /tmp/system-core ./cmd/system-core/`：通过。
- Java 镜像构建开始拉取 `eclipse-temurin:25-jdk`，下载在约 40MB 处长时间没有进展，没有得到编译结果。因此 Java 公开 API、产物读取和版本缓存没有被记为编译通过。
- 端到端演练没有运行。故障闭环、缓存命中、重复事件、旧事件和 Redis 清空重建都没有结果。
- 四条长期分支没有同步。端到端演练没有通过，按计划不执行 fast-forward。

## 未验证

- Compose 全栈启动。
- `deploy/e2e.sh`。
- Kubernetes 清单。
- 国内镜像地址。
