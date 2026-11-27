# 冒烟

按“是否已 up 且迁移完成”选择执行；全部真实依赖 PG/Kafka/域服务，不使用 mock。

```bash
# 1) 聚合入口存活
curl -fsS http://localhost:8080/readyz && echo OK

# 2) 18 路由分段门禁
bash scripts/verify-p6.sh --only=direct    # 7 条直通逐字节 + D-24 结构断言 + 入口唯一
bash scripts/verify-p6.sh --only=bridge    # 旧话题停用 4 断言
bash scripts/verify-p6.sh --only=docs      # 文档最终态 A8/单体/索引

# 3) 结构/搬迁与隔离
bash scripts/verify-p7.sh --structure-only
bash deploy/split/verify-isolation.sh      # 期望 8/8 PASS

# 4) 一键演示（读链）
bash scripts/demo.sh --minimal             # 期望 exit 0
```

> 全链路（monitor→episode→task→agent→artifact）冷启动编排见 `scripts/p6/_coldstart_run.sh`（P6.7 证据）。