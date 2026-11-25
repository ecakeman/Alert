# P1 阶段证据

## 代码基线

| 项 | 值 |
|---|---|
| P1 提交 | P1.1~P1.6（截至 `d608276`） |
| 镜像 | `console-bff:p1-check`（手工构建）；compose 构建标签 `compose-console-bff:latest` |
| 拓扑 | 控制台 → BFF（8080）→ 单体（8081，宿主机） |

## 命令与结果

| # | 检查项 | 命令 | 结果 |
|---|---|---|---|
| 1 | BFF 单测 | `go test ./services/console-bff/...` | PASS（配置/探针/18 路由/404/405/503） |
| 2 | BFF 容器健康 | `docker inspect -f '{{.State.Health.Status}}' alert-console-bff` | healthy |
| 3 | 端口约束 | `docker compose config --format json` | 仅 `console-bff` / `kafka` / `postgres` 暴露端口 |
| 4 | 18+3 路由回归 | `bash scripts/verify-p1-routes.sh` | PASS（21 个响应与 P0 直连基线逐文件一致） |
| 5 | 控制台 9 条主路径 | `bash scripts/smoke-console.sh` | PASS 9/9 |
| 6 | 下游不可达降级 | `SMOKE_DEGRADED=1 bash scripts/smoke-console.sh` | PASS（dashboard 返回 503，不伪装成功） |
| 7 | trace 串联 | 带 `X-Trace-Id` 经 BFF 请求 | BFF 与单体日志各命中同一 trace |
| 8 | P1 门槛 | `bash scripts/verify-p1.sh` | 见本次运行结果 |

## 说明

- P1 阶段 dashboard 与两条 timeline 仍**透明透传**单体；P6 收口时才改为跨域聚合，避免 P1 就引入半成品聚合逻辑。
- `422` 属于 P1 BFF 新错误契约；当前单体对外校验错误仍为 `400`，P2 起逐域切换时再统一。
- 修复了一个既有 bug：创建 target 未携带 `config` 时写入 NULL 导致 500；现默认 `{}`，P0/P1 的 `08_target_create` 基线已同步为 201。

## 复跑方式

```bash
# 1) 起基础设施与 BFF
docker compose -f deploy/compose/compose.yaml up -d postgres kafka console-bff

# 2) 宿主机启动单体（BFF 下游指向 host.docker.internal:8081）
DATABASE_URL='postgres://alert:alert@127.0.0.1:5432/alert?sslmode=disable' \
  KAFKA_BROKERS=127.0.0.1:9092 HTTP_ADDR=:8081 \
  /home/sancho/go-dist/go/bin/go run ./services/system-core/cmd/api

# 3) 门槛
GO_BIN=/home/sancho/go-dist/go/bin/go bash scripts/verify-p1.sh
```

## P1 回退

`bash deploy/split/rollback.sh --to P1 --confirm`：停 BFF、控制台代理指回 `system-core:8080`、恢复 compose 基线；
演练记录见 `deploy/split/README.md` 与 P1.8 提交。