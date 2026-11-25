-- 0004: 摄取任务与观测（ingest_jobs / llm_calls）
-- ingest_jobs 是摄取状态的持久化权威（CR-ING-02），禁止进程内 dict 充当状态；
-- error_msg 上限 500 字符且不含凭据/完整路径（7.5.6）。
-- llm_calls 为观测表：主链路只投递内存有界队列，后台批量 flush 落库（CR-REG-19）。
-- 纯 SQL 裸迁移：不含 BEGIN/COMMIT；禁止 DROP；无 CASE WHEN；整体执行（CR-REG-37）。

CREATE TABLE cognitive.ingest_jobs (
    doc_id       text        NOT NULL,
    status       text        NOT NULL DEFAULT 'QUEUED',
    chunk_count  integer,
    error_msg    text        NOT NULL DEFAULT '',
    callback_url text        NOT NULL,
    attempt      integer     NOT NULL DEFAULT 0,
    file_path    text        NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT ingest_jobs_pk PRIMARY KEY (doc_id),
    CONSTRAINT ingest_jobs_status_check CHECK (status IN ('QUEUED','PROCESSING','READY','FAILED')),
    CONSTRAINT ingest_jobs_chunk_count_check CHECK (chunk_count IS NULL OR chunk_count >= 0),
    CONSTRAINT ingest_jobs_error_len_check CHECK (char_length(error_msg) <= 500),
    CONSTRAINT ingest_jobs_attempt_check CHECK (attempt >= 0)
);
CREATE INDEX ingest_jobs_status_idx ON cognitive.ingest_jobs (status, updated_at);

CREATE TABLE cognitive.llm_calls (
    call_id           text        NOT NULL,
    scene             text        NOT NULL,
    model             text        NOT NULL,
    latency_ms        bigint      NOT NULL,
    prompt_tokens     integer     NOT NULL DEFAULT 0,
    completion_tokens integer     NOT NULL DEFAULT 0,
    degraded          boolean     NOT NULL DEFAULT false,
    ts                timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT llm_calls_pk PRIMARY KEY (call_id),
    CONSTRAINT llm_calls_scene_len_check CHECK (char_length(scene) BETWEEN 1 AND 64),
    CONSTRAINT llm_calls_latency_check CHECK (latency_ms >= 0),
    CONSTRAINT llm_calls_tokens_check CHECK (prompt_tokens >= 0 AND completion_tokens >= 0)
);
CREATE INDEX llm_calls_ts_idx ON cognitive.llm_calls (ts DESC);