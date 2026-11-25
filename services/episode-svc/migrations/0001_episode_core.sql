-- 0001: episode 本域基线（episodes / input_versions / app_events / outbox / idempotency）
-- service_id 为逻辑引用（删跨域 FK）；episode_id 同域外键保留。
-- 纯 SQL：不含 BEGIN/COMMIT；禁止 DROP/CREATE SCHEMA。

CREATE TABLE episode.episodes (
    episode_id              uuid PRIMARY KEY,
    service_id              uuid NOT NULL,
    status                  text NOT NULL DEFAULT 'ACTIVE',
    started_by              text NOT NULL,
    resolution_pending      boolean NOT NULL DEFAULT false,
    input_hash              text NOT NULL DEFAULT '',
    input_version           bigint NOT NULL DEFAULT 0,
    processed_input_version bigint NOT NULL DEFAULT 0,
    final_input_version     bigint NOT NULL DEFAULT 0,
    monitor_watermark       bigint NOT NULL DEFAULT 0,
    app_event_watermark     bigint NOT NULL DEFAULT 0,
    started_at              timestamptz NOT NULL DEFAULT now(),
    resolved_at             timestamptz,
    archived_at             timestamptz,
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT episodes_status_check CHECK (status IN ('ACTIVE','RESOLVED','ARCHIVED')),
    CONSTRAINT episodes_started_by_check CHECK (started_by IN ('MONITOR','APP')),
    CONSTRAINT episodes_resolved_time_check CHECK (resolved_at IS NULL OR resolved_at >= started_at),
    CONSTRAINT episodes_archived_time_check CHECK (archived_at IS NULL OR (resolved_at IS NOT NULL AND archived_at >= resolved_at)),
    CONSTRAINT episodes_resolved_consistency_check CHECK (status <> 'RESOLVED' OR resolved_at IS NOT NULL),
    CONSTRAINT episodes_archived_consistency_check CHECK (status <> 'ARCHIVED' OR archived_at IS NOT NULL),
    CONSTRAINT episodes_processed_version_check CHECK (processed_input_version <= input_version),
    CONSTRAINT episodes_final_version_check CHECK (final_input_version <= input_version)
);
CREATE UNIQUE INDEX episodes_one_active ON episode.episodes (service_id) WHERE status = 'ACTIVE';
CREATE INDEX episodes_service_idx ON episode.episodes (service_id, started_at DESC);
CREATE INDEX episodes_status_idx ON episode.episodes (status, started_at DESC);

CREATE TABLE episode.episode_input_versions (
    episode_id          uuid NOT NULL REFERENCES episode.episodes(episode_id) ON DELETE RESTRICT,
    input_version       bigint NOT NULL,
    input_hash          text NOT NULL,
    monitor_projection  jsonb NOT NULL DEFAULT '[]'::jsonb,
    monitor_watermark   bigint NOT NULL DEFAULT 0,
    app_event_watermark bigint NOT NULL DEFAULT 0,
    created_at          timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (episode_id, input_version),
    CONSTRAINT input_versions_version_check CHECK (input_version > 0),
    CONSTRAINT episode_input_versions_hash UNIQUE (episode_id, input_hash)
);

CREATE TABLE episode.episode_app_events (
    event_record_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    app_event_seq   bigint NOT NULL,
    service_id      uuid NOT NULL,
    episode_id      uuid REFERENCES episode.episodes(episode_id) ON DELETE RESTRICT,
    producer_id     text NOT NULL,
    event_id        text NOT NULL,
    event_kind      text NOT NULL DEFAULT '',
    health_status   text NOT NULL DEFAULT '',
    payload         jsonb NOT NULL DEFAULT '{}'::jsonb,
    content_hash    text NOT NULL,
    event_time      timestamptz NOT NULL,
    received_at     timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT episode_app_events_identity UNIQUE (service_id, producer_id, event_id),
    CONSTRAINT episode_app_events_seq UNIQUE (service_id, app_event_seq)
);
CREATE INDEX app_events_service_idx ON episode.episode_app_events (service_id, app_event_seq DESC);
CREATE INDEX app_events_episode_idx ON episode.episode_app_events (episode_id);

CREATE TABLE episode.outbox_events (
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
    ON episode.outbox_events (dedupe_key)
    WHERE dedupe_key IS NOT NULL;
CREATE INDEX outbox_pending_idx ON episode.outbox_events (next_retry_at, created_at) WHERE published_at IS NULL;
CREATE INDEX outbox_claim_idx ON episode.outbox_events (claim_until) WHERE published_at IS NULL;

CREATE TABLE episode.consumer_idempotency (
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
CREATE INDEX consumer_idem_processed_idx ON episode.consumer_idempotency (processed_at);