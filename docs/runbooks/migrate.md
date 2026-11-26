# Runbook：迁移回放与验证

工作目录：`go/services/system-core`。

## 1. 常规回放

```bash
DATABASE_URL=postgres://alert:<密码>@127.0.0.1:5432/alert?sslmode=disable \
KAFKA_BROKERS=127.0.0.1:9092 \
go run ./cmd/api -migrate
```

- 首轮：`applied=3 skipped=0`（对空库）。
- 次轮：no-op，`applied=0 skipped=3`——两次回放一致才算通过。
- 缺 `DATABASE_URL` 或 `KAFKA_BROKERS` 时在配置校验处非零退出，属预期失败路径。

## 2. 专用库验证（不动开发库）

```bash
psql -h 127.0.0.1 -U alert -d postgres -c 'CREATE DATABASE <专用库名>;'
# 用上面 DATABASE_URL 指向专用库名执行两次 -migrate
```

## 3. 常见失败

| 现象 | 处理 |
|---|---|
| checksum conflict | 说明迁移文件被改动；恢复文件原文，不改 checksum 记录 |
| 版本缺口/重复 | runner 校验拒绝；检查 `migrations/` 目录文件序 |
| 连接失败 | 确认 compose 拓扑与账号，勿把示例密码当真实凭据 |