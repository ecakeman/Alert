# Alert

面向服务故障的监控、调查与可追溯诊断系统（重建中）。

本仓库为全新工程骨架：Go 模块化单体（Gin 对外 API），后续接入 Python Agent Runtime 与 Vue 控制台。
当前仅搭骨架，不含业务实现，也不连接真实外部依赖。

## 环境

- Go 1.27.x（本机使用 `~/.local/go`，命令前 `export PATH=$HOME/.local/go/bin:$PATH`）
- 后续：Node 22（前端）、Docker（容器化）

## 启动（骨架完成阶段）

```
go run ./cmd/system-core
```

配置项见 `.env.example`（7 项，均有默认值，缺省即可启动）。

## 验收命令（骨架完成后可用）

- `go build ./...`
- `go list -m`
- `go test ./internal/domain/...`
- `curl http://127.0.0.1:8080/healthz`
- `curl http://127.0.0.1:8080/readyz`
- `curl -i http://127.0.0.1:8080/api/v1/services`（未实现路由应返回 501）
- `cd apps/console && npm run build`

## 范围声明

仅用于本地 / 可信内网演示，未接入鉴权与真实依赖，不代表生产可用。