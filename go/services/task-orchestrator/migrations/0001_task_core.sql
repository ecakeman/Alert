-- 0001: task 域核心表（agent_tasks / ownership / artifact_refs / fault_document_refs / outbox / idempotency）
-- 跨域引用（episode_id / service_id）一律为逻辑引用，不建跨 schema FK；同域 FK 保留在 task schema 内。
-- 纯 SQL：不含 BEGIN/COMMIT；禁止 DROP。


CREATE TABLE task.agent_tasks (
    task_id              uuid PRIMARY KEY,
    episode_id           uuid NOT NULL,
    service_id           uuid NOT NULL,
    task_type            text NOT NULL,
    status               text NOT NULL DEFAULT 'PENDING',
    generation           bigint NOT NULL DEFAULT 1,
    attempt              integer NOT NULL DEFAULT 0,
    max_attempts         integer NOT NULL DEFAULT 3,
    lease_until          timestamptz,
    next_retry_at        timestamptz,
    input_version_from   bigint NOT NULL,
    input_version_to     bigint NOT NULL,
    monitor_watermark    bigint NOT NULL,
    app_event_watermark  bigint NOT NULL,
    deadline_at          timestamptz NOT NULL,
    error_code           text NOT NULL DEFAULT '',
    error_message        text NOT NULL DEFAULT '',
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now(),
    started_at           timestamptz,
    finished_at          timestamptz,
    CONSTRAINT tasks_type_check CHECK (task_type IN ('DIAGNOSIS','ARCHIVE')),
    CONSTRAINT tasks_status_check CHECK (status IN ('PENDING','DISPATCHED','RUNNING','RETRY_WAIT','SUCCEEDED','FAILED')),
    CONSTRAINT tasks_generation_check CHECK (generation > 0),
    CONSTRAINT tasks_attempt_check CHECK (attempt >= 0),
    CONSTRAINT tasks_max_attempts_check CHECK (max_attempts > 0),
    CONSTRAINT tasks_version_range_check CHECK (input_version_from > 0 AND input_version_from <= input_version_to),
    CONSTRAINT tasks_watermark_check CHECK (monitor_watermark >= 0 AND app_event_watermark >= 0),
    CONSTRAINT tasks_retry_wait_check CHECK (status <> 'RETRY_WAIT' OR next_retry_at IS NOT NULL)
);
CREATE UNIQUE INDEX agent_tasks_one_active
    ON task.agent_tasks (episode_id)
    WHERE status IN ('PENDING','DISPATCHED','RUNNING','RETRY_WAIT');
CREATE INDEX tasks_episode_idx ON task.agent_tasks (episode_id, created_at DESC);
CREATE INDEX tasks_status_idx ON task.agent_tasks (status, next_retry_at);
CREATE INDEX tasks_service_idx ON task.agent_tasks (service_id, created_at DESC);

CREATE TABLE task.service_execution_ownership (
    service_id  uuid PRIMARY KEY,
    task_id     uuid NOT NULL REFERENCES task.agent_tasks(task_id) ON DELETE RESTRICT,
    episode_id  uuid NOT NULL,
    task_type   text NOT NULL,
    generation  bigint NOT NULL,
    lease_until timestamptz NOT NULL,
    acquired_at timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT ownership_type_check CHECK (task_type IN ('DIAGNOSIS','ARCHIVE')),
    CONSTRAINT ownership_generation_check CHECK (generation > 0),
    CONSTRAINT ownership_lease_check CHECK (lease_until > acquired_at)
);
CREATE INDEX ownership_lease_idx ON task.service_execution_ownership (lease_until);

CREATE TABLE task.outbox_events (
    event_id          uuid PRIMARY KEY,
    event_type        text NOT NULL,
    schema_version    integer NOT NULL DEFAULT 1,
    aggregate_type    text NOT NULL,
    aggregate_id      text NOT NULL,
    payload           jsonb NOT NULL,
    dedupe_key        text,
    created_at        timestamptz NOT NULL DEFAULT now(),
    published_at      timestamptz,
    publish_attempts  integer NOT NULL DEFAULT 0,
    next_retry_at     timestamptz,
    claim_owner       text,
    claim_until       timestamptz,
    last_error        text NOT NULL DEFAULT '',
    CONSTRAINT outbox_schema_version_check CHECK (schema_version > 0),
    CONSTRAINT outbox_publish_attempts_check CHECK (publish_attempts >= 0),
    CONSTRAINT outbox_published_time_check CHECK (published_at IS NULL OR published_at >= created_at),
    CONSTRAINT outbox_claim_check CHECK (claim_until IS NULL OR claim_owner IS NOT NULL)
);
CREATE UNIQUE INDEX outbox_events_dedupe_key
    ON task.outbox_events (dedupe_key)
    WHERE dedupe_key IS NOT NULL;
CREATE INDEX outbox_pending_idx ON task.outbox_events (next_retry_at, created_at) WHERE published_at IS NULL;
CREATE INDEX outbox_claim_idx ON task.outbox_events (claim_until) WHERE published_at IS NULL;

CREATE TABLE task.consumer_idempotency (
    consumer        text NOT NULL,
    idempotency_key text NOT NULL,
    event_id        uuid,
    task_id         uuid,
    generation      bigint,
    result_code     text NOT NULL DEFAULT '',
    processed_at    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (consumer, idempotency_key),
    CONSTRAINT consumer_idem_target_check CHECK (event_id IS NOT NULL OR (task_id IS NOT NULL AND generation IS NOT NULL))
);
CREATE INDEX consumer_idem_processed_idx ON task.consumer_idempotency (processed_at);

CREATE TABLE task.episode_artifact_refs (
    artifact_ref_id  uuid PRIMARY KEY,
    episode_id       uuid NOT NULL,
    task_id          uuid NOT NULL REFERENCES task.agent_tasks(task_id) ON DELETE RESTRICT,
    generation       bigint NOT NULL,
    artifact_type    text NOT NULL,
    artifact_version integer NOT NULL,
    content_hash     text NOT NULL,
    content_type     text NOT NULL DEFAULT 'text/markdown',
    storage_key      text NOT NULL,
    source_refs      jsonb NOT NULL DEFAULT '[]'::jsonb,
    registered_at    timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT artifact_type_check CHECK (artifact_type IN ('INVESTIGATION','DIAGNOSIS_REPORT')),
    CONSTRAINT artifact_version_check CHECK (artifact_version > 0),
    CONSTRAINT artifact_generation_check CHECK (generation > 0),
    CONSTRAINT episode_artifact_refs_version UNIQUE (episode_id, artifact_type, artifact_version)
);
CREATE INDEX artifact_refs_lookup_idx ON task.episode_artifact_refs (episode_id, artifact_type, artifact_version DESC);

CREATE TABLE task.episode_fault_document_refs (
    doc_ref_id       uuid PRIMARY KEY,
    episode_id       uuid NOT NULL,
    task_id          uuid REFERENCES task.agent_tasks(task_id) ON DELETE RESTRICT,
    generation       bigint,
    document_type    text NOT NULL,
    document_version integer NOT NULL,
    content_hash     text NOT NULL,
    storage_key      text NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT fault_doc_type_check CHECK (document_type = 'FAULT_DOC'),
    CONSTRAINT fault_doc_version_check CHECK (document_version > 0),
    CONSTRAINT fault_doc_generation_check CHECK (generation IS NULL OR generation > 0),
    CONSTRAINT episode_fault_document_refs_version UNIQUE (episode_id, document_type, document_version)
);