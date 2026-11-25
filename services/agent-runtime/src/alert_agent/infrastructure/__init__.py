"""基础设施层：实现应用层端口的真实适配器。

包含 Postgres 仓储/迁移、OpenAI 兼容的 LLM/Embedding/Rerank 客户端、gRPC
生成桩（``grpc/gen``）及网关客户端、MCP/A2A 客户端、产物文件存储、进程内
BM25 索引、后台观测器。基础设施只实现 ``application`` 端口，绝不能被
domain/application 向上导入（CR-LAY-01~03）。
"""