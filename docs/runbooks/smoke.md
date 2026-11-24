# Runbook：冒烟路径（S5 执行，此处为命令清单）

前置：`deploy/compose` 拓扑已起、迁移已回放、api + worker 双进程已启动。工作目录不限命令者；全部为 HTTP 调用。

## 1. 探针

```bash
curl -s http://127.0.0.1:8080/healthz
curl -s http://127.0.0.1:8080/readyz
```

## 2. 业务路由 1~15（前缀 `http://127.0.0.1:8080/api/v1`）

| # | 方法与路径 |
|---|---|
| 1 | `GET /dashboard/summary` |
| 2 | `GET /services` |
| 3 | `POST /services` |
| 4 | `GET /services/:service_id` |
| 5 | `PATCH /services/:service_id` |
| 6 | `DELETE /services/:service_id` |
| 7 | `GET /services/:service_id/targets` |
| 8 | `POST /services/:service_id/targets` |
| 9 | `PATCH /targets/:target_id` |
| 10 | `DELETE /targets/:target_id` |
| 11 | `GET /episodes` |
| 12 | `GET /episodes/:episode_id` |
| 13 | `POST /services/:service_id/events` |
| 14 | `GET /services/:service_id/timeline` |
| 15 | `GET /episodes/:episode_id/timeline` |

## 3. 保留 501 路由（16~18，B7 待接线）

```bash
curl -i http://127.0.0.1:8080/api/v1/episodes/<episode_id>/artifacts
curl -i -X POST http://127.0.0.1:8080/api/v1/episodes/<episode_id>/retry-diagnosis
curl -i -X POST http://127.0.0.1:8080/api/v1/episodes/<episode_id>/retry-archive
```

## 4. 探测路径

1. 建服务 + 建启用 target（HTTP 型，`interval_ms` 调小）。
2. 等 worker 调度：`GET /services/:service_id/timeline` 观察监控点与状态聚合。
3. Kafka 侧确认 `alertflow.system.events.v1` 有事件产出。