# 本轮三域边界

本文只描述这一轮整改。开始时 `main` 是 `3d84505b8cc4cfdbfb006861ad2cb0ec47c501c3`。`backend`、`agent`、`frontend` 是 `9c6223189238d35a545eb5cffb27b224da7dbd0a`，`integration` 是 `cf59bea10925e019c9fa3ede58eb0a4b0917e470`。其余分支后续继续维护，这里不规定它们的日常流程。

四个运行区域：

- Java 是唯一公开 REST 入口，并转发到 Go 和 Python。
- Go 拥有监控、Episode、Timeline、Task 和 Outbox。它不再对宿主机发布 HTTP。
- Python 做诊断和归档。它不写 Go 的系统表，时间轴和任务回报经过 Java。
- Vue 只访问 Java 的公开 API。

监控用进程内的有界队列。Agent 任务仍走 PostgreSQL Outbox 和 Kafka。两者不是同一套队列。

Redis 只给 Java 做带版本的查询缓存。Go 和 Python 不把 Redis 当业务事实或任务队列。
