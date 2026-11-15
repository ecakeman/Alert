# 0001 Java 成为唯一公开 REST

本轮把现有公开路径从 Go 的 Gin 迁到 Java。路径、方法和 JSON 保持不变。Go 仍执行业务事务。Java 不读写 Go 的 system 表。宿主机 `18080` 改映射 Java 的 `8081`。
