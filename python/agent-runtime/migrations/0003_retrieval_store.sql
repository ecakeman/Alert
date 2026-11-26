-- 0003: RAG 检索库（retrieval_documents / retrieval_chunks）
-- is_parent 显式列用于 BM25/向量腿排除父块（CR-RAG-08、CR-REG-24）；
-- embedding 为无维度 vector（D7 应用层校验维度一致性）。
-- retrieval_documents.status 的取值（QUEUED/PROCESSING/READY/FAILED）由应用层维护，
-- 与 ingest_jobs 状态机一致；本迁移不定 CHECK（design §6.2 未定，forward-only 保守）。
-- 纯 SQL 裸迁移：不含 BEGIN/COMMIT；禁止 DROP；无 CASE WHEN；整体执行（CR-REG-37）。

CREATE TABLE cognitive.retrieval_documents (
    doc_id     text        NOT NULL,
    title      text        NOT NULL,
    status     text        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT retrieval_documents_pk PRIMARY KEY (doc_id)
);
CREATE INDEX retrieval_documents_status_idx ON cognitive.retrieval_documents (status);

CREATE TABLE cognitive.retrieval_chunks (
    chunk_id     text        NOT NULL,
    doc_id       text        NOT NULL REFERENCES cognitive.retrieval_documents (doc_id) ON DELETE CASCADE,
    parent_id    text,
    is_parent    boolean     NOT NULL DEFAULT false,
    chunk_index  integer     NOT NULL,
    content      text        NOT NULL,
    section_path text,
    page         integer,
    embedding    vector,
    created_at   timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT retrieval_chunks_pk PRIMARY KEY (chunk_id),
    CONSTRAINT retrieval_chunks_index_check CHECK (chunk_index >= 0),
    CONSTRAINT retrieval_chunks_parent_check CHECK (parent_id <> chunk_id)
);
CREATE INDEX retrieval_chunks_doc_idx ON cognitive.retrieval_chunks (doc_id, is_parent);
CREATE INDEX retrieval_chunks_parent_idx ON cognitive.retrieval_chunks (parent_id) WHERE parent_id IS NOT NULL;