# Runbook：本地启动（双终端）

## 1. 依赖拓扑（工作目录：仓库根）

```bash
docker compose -f deploy/compose/compose.yaml up -d
```

提供 PG16（127.0.0.1:5432，alert/alert）与 Kafka KRaft（127.0.0.1:9092）。配置基线见 `.env.example`（33 项，均有默认值）。

## 2. 迁移（工作目录：`go/services`，首次或 schema 变更后）

```bash
各域服务 -migrate（见 deploy/compose）
```

## 3. 双进程（`go/services` 内，两个终端分别执行）

```bash
console-bff / 域服务      # REST + 探针，默认 :8080
域服务后台循环   # 探测调度 / outbox 发布 / consumer
```

## 4. 就绪确认

```bash
curl -s http://127.0.0.1:8080/healthz
curl -s http://127.0.0.1:8080/readyz
```

`readyz` 通过即 DB 连接池与 Kafka 客户端就绪。退出方式：api 用 `HTTP_SHUTDOWN_TIMEOUT` 内的优雅停机；worker 停止接收新样本后等待在途完成。