# 0003 Redis 只保存带版本的 API 缓存

本轮去掉 Go 进程里的 Redis 读缓存。Go 在业务事务中推进资源版本，并用独立 Topic `alertflow.system-resource-revision-changed.v1` 通知 Java。Java 只把版本指针向前推进。Timeline 和产物不进入这批缓存。Redis 不可用时，Java 直接查询 Go。
