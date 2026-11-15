# 0002 Go 使用有界监控执行

本轮替换每 200 毫秒全量扫描、队列满就丢弃的循环。调度用到期最小堆，队列有上限，同一目标不同时排队和执行。队列满时目标留在堆里。HTTP 客户端在进程内复用，默认校验证书。Kubernetes 仍只观察 Deployment 和 StatefulSet。MONITOR、APP、单一 ACTIVE Episode 和双来源恢复的规则不改。
