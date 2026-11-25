-- 0001: 任务回执与产物登记（agent_task_receipts / episode_artifacts）
-- cognitive schema（Python 独占写，CR-DATA-01/02）：与 system 迁移完全分离，
-- Go 迁移 runner 不发现不执行本目录（CR-DATA-02）。
-- 纯 SQL 裸迁移：不含 BEGIN/COMMIT（runner 单文件单事务）；禁止 DROP；
-- 无 CASE WHEN 决定状态；禁止按 ';' 拆分，runner 整体执行（CR-REG-37）。
-- 向量列不固定维度（D7）：实际维度由 embed_dim 配置在应用层校验（启动期一致性校验）。

CREATE SCHEMA IF NOT EXISTS cognitive;

CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE cognitive.agent_task_receipts (
    task_id            text        NOT NULL,
    generation         bigint      NOT NULL,
    task_type          text        NOT NULL,
    episode_id         text        NOT NULL,
    service_id         text        NOT NULL,
    thread_id          text        NOT NULL,
    status             text        NOT NULL DEFAULT 'ACCEPTED',
    callback_state     text        NOT NULL DEFAULT 'NOT_SENT',
    last_error_code    text        NOT NULL DEFAULT '',
    last_error_message text        NOT NULL DEFAULT '',
    accepted_at        timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    completed_at       timestamptz,
    CONSTRAINT agent_task_receipts_pk PRIMARY KEY (task_id, generation),
    CONSTRAINT agent_task_receipts_type_check CHECK (task_type IN ('DIAGNOSIS','ARCHIVE')),
    CONSTRAINT agent_task_receipts_generation_check CHECK (generation > 0),
    CONSTRAINT agent_task_receipts_status_check CHECK (status IN ('ACCEPTED','RUNNING','COMPLETED','FAILED')),
    CONSTRAINT agent_task_receipts_callback_check CHECK (callback_state IN ('NOT_SENT','SENT','CONFIRMED'))
);
CREATE INDEX agent_task_receipts_unfinished_idx
    ON cognitive.agent_task_receipts (status)
    WHERE status IN ('ACCEPTED','RUNNING');
CREATE INDEX agent_task_receipts_episode_idx ON cognitive.agent_task_receipts (episode_id);

CREATE TABLE cognitive.episode_artifacts (
    artifact_id      text        NOT NULL,
    task_id          text        NOT NULL,
    generation       bigint      NOT NULL,
    episode_id       text        NOT NULL,
    artifact_type    text        NOT NULL,
    artifact_version integer     NOT NULL,
    content_hash     text        NOT NULL,
    content_type     text        NOT NULL DEFAULT 'text/markdown',
    storage_key      text        NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT episode_artifacts_pk PRIMARY KEY (artifact_id),
    CONSTRAINT episode_artifacts_type_check CHECK (artifact_type IN ('INVESTIGATION','DIAGNOSIS_REPORT')),
    CONSTRAINT episode_artifacts_version_check CHECK (artifact_version > 0),
    CONSTRAINT episode_artifacts_generation_check CHECK (generation > 0),
    CONSTRAINT episode_artifacts_identity UNIQUE (episode_id, artifact_type, artifact_version)
);
CREATE INDEX episode_artifacts_task_idx ON cognitive.episode_artifacts (task_id, generation);