-- 0002: 故障知识文档与分块（fault_documents / fault_doc_chunks）
-- 四字段检索索引 summary / symptoms / observations / diagnosis（CR-ARCH-03/05），
-- symptoms 与 observations 的语义差异由应用层校验（CR-ARCH-03）。
-- tsv 全文腿列由应用层填充（D9 备选评估点）；embedding 为无维度 vector（D7 应用层校验）。
-- 纯 SQL 裸迁移：不含 BEGIN/COMMIT；禁止 DROP；无 CASE WHEN；整体执行（CR-REG-37）。

CREATE TABLE cognitive.fault_documents (
    fd_id        text        NOT NULL,
    episode_id   text        NOT NULL,
    service_id   text        NOT NULL,
    task_id      text        NOT NULL,
    generation   bigint      NOT NULL,
    summary      text        NOT NULL,
    symptoms     jsonb       NOT NULL,
    observations jsonb       NOT NULL,
    diagnosis    jsonb       NOT NULL,
    content_hash text        NOT NULL,
    index_status text        NOT NULL DEFAULT 'BUILDING',
    index_error  text        NOT NULL DEFAULT '',
    embedding    vector,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT fault_documents_pk PRIMARY KEY (fd_id),
    CONSTRAINT fault_documents_episode_key UNIQUE (episode_id),
    CONSTRAINT fault_documents_status_check CHECK (index_status IN ('BUILDING','READY','FAILED')),
    CONSTRAINT fault_documents_generation_check CHECK (generation > 0)
);
CREATE INDEX fault_documents_service_idx ON cognitive.fault_documents (service_id, index_status);

CREATE TABLE cognitive.fault_doc_chunks (
    chunk_id    text        NOT NULL,
    fd_id       text        NOT NULL REFERENCES cognitive.fault_documents (fd_id) ON DELETE CASCADE,
    chunk_index integer     NOT NULL,
    content     text        NOT NULL,
    tsv         tsvector,
    embedding   vector,
    created_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT fault_doc_chunks_pk PRIMARY KEY (chunk_id),
    CONSTRAINT fault_doc_chunks_index_check CHECK (chunk_index >= 0)
);
CREATE INDEX fault_doc_chunks_fd_idx ON cognitive.fault_doc_chunks (fd_id, chunk_index);
CREATE INDEX fault_doc_chunks_tsv_idx ON cognitive.fault_doc_chunks USING gin (tsv);
-- 注：embedding 为无维度 vector（D7），pgvector 无法对其建 HNSW/IVFFlat 索引；
-- 召回走顺序扫描，维度一致性由 embed_dim 配置校验承接。