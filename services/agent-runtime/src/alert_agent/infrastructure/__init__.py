"""Infrastructure layer: real adapters implementing application ports.

Postgres repositories/migrations, OpenAI-compatible LLM/Embedding/Rerank
clients, gRPC generated stubs (``grpc/gen``) plus gateway client, MCP/A2A
clients, artifact file store, in-process BM25 index, background observers.
Infrastructure implements ``application`` ports only and must never be
imported by domain/application upward (CR-LAY-01~03).
"""