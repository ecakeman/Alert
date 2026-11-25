"""摄取流水线基础设施（T20，CR-ING-03/04）。

``DeterministicChunker``：基于规则的父/子分块（两遍式：小节聚合为上限约
900 token 的父块，子块按 chunk_size_tokens 带重叠切分；父块承载完整小节）。
分块路径中无 LLM（CR-DISC-05 对摄取的纪律）。``IngestFileLoader`` 读取
摄取根目录内的文档正文（复用 T19 路径规则）。``PostgresIngestionWriter``：
先清理上一版本，再在单个事务中写入文档及带嵌入的父/子块——失败不会留下
半摄取的可检索数据（CR-ING-04）。
"""

from __future__ import annotations

from pathlib import Path

from psycopg_pool import AsyncConnectionPool

from alert_agent.application.ingestion.ports import DocumentChunk
from alert_agent.domain.ingestion.path_rules import resolve_within_root
from alert_agent.infrastructure.embedding import OpenAiCompatibleEmbedder
from alert_agent.infrastructure.llm.client import OpenAiCompatibleLlmClient

_CHARS_PER_TOKEN = 4


class DeterministicChunker:
    """``Chunker`` 端口实现（T20）。"""

    def __init__(self, *, chunk_size_tokens: int = 500, overlap_tokens: int = 50) -> None:
        self._chunk_chars = max(1, chunk_size_tokens * _CHARS_PER_TOKEN)
        self._overlap_chars = max(0, overlap_tokens * _CHARS_PER_TOKEN)

    def chunk(self, content: str) -> list[DocumentChunk]:
        sections = [section.strip() for section in content.split("\n\n") if section.strip()]
        if not sections:
            sections = [content.strip()] if content.strip() else []
        result: list[DocumentChunk] = []
        child_index = 0
        for parent_index, section in enumerate(sections):
            parent_id = f"p{parent_index}"
            parent_content = section
            result.append(
                DocumentChunk(
                    parent_id=None,
                    is_parent=True,
                    chunk_index=parent_index,
                    content=parent_content,
                    section_path=f"section-{parent_index}",
                )
            )
            # 父块字符预算内的子块
            step = max(1, self._chunk_chars - self._overlap_chars)
            position = 0
            while position < len(section):
                piece = section[position : position + self._chunk_chars].strip()
                if piece:
                    result.append(
                        DocumentChunk(
                            parent_id=parent_id,
                            is_parent=False,
                            chunk_index=child_index,
                            content=piece,
                            section_path=f"section-{parent_index}",
                        )
                    )
                    child_index += 1
                if position + self._chunk_chars >= len(section):
                    break
                position += step
        return result


class IngestFileLoader:
    """``ContentLoader`` 端口实现（T20）：强制执行路径规则。"""

    def __init__(self, root: Path) -> None:
        self._root = root.resolve()

    async def load(self, file_path: str) -> str:
        # PathRuleError 原样传播（传输层将其映射为 400）。
        target = resolve_within_root(self._root, file_path)
        if not target.is_file():
            raise FileNotFoundError(f"ingest file missing: {file_path}")
        return target.read_text(encoding="utf-8")


_DELETE_DOC = "DELETE FROM cognitive.retrieval_chunks WHERE doc_id = %s"
_DELETE_DOC_ROW = "DELETE FROM cognitive.retrieval_documents WHERE doc_id = %s"
_UPSERT_DOC = """
INSERT INTO cognitive.retrieval_documents (doc_id, title, status)
VALUES (%s, %s, 'READY')
ON CONFLICT (doc_id) DO UPDATE SET title = EXCLUDED.title, status = 'READY', updated_at = now()
"""
_INSERT_CHUNK = """
INSERT INTO cognitive.retrieval_chunks
    (chunk_id, doc_id, parent_id, is_parent, chunk_index, content, section_path, page, embedding)
VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s::vector)
"""


def _vector_literal(embedding: list[float] | None) -> str | None:
    if embedding is None:
        return None
    return "[" + ",".join(f"{component:.6f}" for component in embedding) + "]"


class PostgresIngestionWriter:
    """``IngestionWriter`` 端口实现（T20）。"""

    def __init__(
        self,
        pool: AsyncConnectionPool,
        *,
        embedder: OpenAiCompatibleEmbedder | OpenAiCompatibleLlmClient | None = None,
    ) -> None:
        self._pool = pool
        self._embedder = embedder  # 为对称装配保留的未使用占位

    async def write(
        self,
        *,
        doc_id: str,
        title: str,
        chunks: list[DocumentChunk],
        embeddings: list[list[float] | None],
    ) -> int:
        if len(embeddings) != len(chunks):
            raise ValueError("embeddings and chunks must align 1:1")
        async with self._pool.connection() as connection, connection.transaction():
            # 幂等重跑：先清理上一版数据（CR-ING-03）。
            await connection.execute(_DELETE_DOC, (doc_id,))
            await connection.execute(_DELETE_DOC_ROW, (doc_id,))
            await connection.execute(_UPSERT_DOC, (doc_id, title[:500]))
            written = 0
            for chunk, embedding in zip(chunks, embeddings):
                chunk_id = f"{doc_id}:{'p' if chunk.is_parent else 'c'}{chunk.chunk_index}"
                await connection.execute(
                    _INSERT_CHUNK,
                    (
                        chunk_id,
                        doc_id,
                        chunk.parent_id,
                        chunk.is_parent,
                        chunk.chunk_index,
                        chunk.content,
                        chunk.section_path,
                        chunk.page,
                        _vector_literal(embedding),
                    ),
                )
                written += 1
        return written

    async def delete_document(self, doc_id: str) -> bool:
        async with self._pool.connection() as connection, connection.transaction():
            cursor = await connection.execute(_DELETE_DOC, (doc_id,))
            chunks_deleted = cursor.rowcount > 0
            cursor = await connection.execute(_DELETE_DOC_ROW, (doc_id,))
            doc_deleted = cursor.rowcount > 0
        return chunks_deleted or doc_deleted