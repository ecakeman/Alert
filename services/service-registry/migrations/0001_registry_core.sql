-- 0001: registry 本域基线（services / targets / source_states / outbox / idempotency）
-- 表体从 system 0001+0003 复制；删除跨域 FK，保留全部 CHECK 与部分唯一索引。
-- 纯 SQL：不含 BEGIN/COMMIT；禁止 DROP/CREATE SCHEMA。

CREATE TABLE registry.services (
    service_id               uuid PRIMARY KEY,
    service_key              text NOT NULL,
    name                     text NOT NULL,
    description              text NOT NULL DEFAULT '',
    enabled                  boolean NOT NULL DEFAULT true,
    monitor_aggregation_mode text NOT NULL DEFAULT 'ALL_REQUIRED',
    authority_producer_id    text NOT NULL,
    deleted_at               timestamptz,
    created_at               timestamptz NOT NULL DEFAULT now(),
    updated_at               timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT services_service_key_key UNIQUE (service_key),
    CONSTRAINT services_key_len_check CHECK (char_length(service_key) BETWEEN 1 AND 64),
    CONSTRAINT services_name_len_check CHECK (char_length(name) BETWEEN 1 AND 200),
    CONSTRAINT services_authority_len_check CHECK (char_length(authority_producer_id) BETWEEN 1 AND 128),
    CONSTRAINT services_aggregation_check CHECK (monitor_aggregation_mode IN ('ALL_REQUIRED','ANY_REQUIRED','MAJORITY'))
);
CREATE INDEX services_enabled_idx ON registry.services (enabled, deleted_at);

CREATE TABLE registry.service_monitor_targets (
    target_id             uuid PRIMARY KEY,
    service_id            uuid NOT NULL REFERENCES registry.services(service_id) ON DELETE RESTRICT,
    target_key            text NOT NULL,
    target_name           text NOT NULL,
    address               text NOT NULL,
    target_type           text NOT NULL DEFAULT 'HTTP',
    config                jsonb NOT NULL DEFAULT '{}'::jsonb,
    interval_ms           integer NOT NULL,
    timeout_ms            integer NOT NULL,
    failure_threshold     integer NOT NULL DEFAULT 3,
    recovery_threshold    integer NOT NULL DEFAULT 2,
    is_required           boolean NOT NULL DEFAULT true,
    enabled               boolean NOT NULL DEFAULT true,
    status                text NOT NULL DEFAULT 'UNKNOWN',
    consecutive_failures  integer NOT NULL DEFAULT 0,
    consecutive_successes integer NOT NULL DEFAULT 0,
    last_check_at         timestamptz,
    config_generation     bigint NOT NULL DEFAULT 1,
    deleted_at            timestamptz,
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT targets_key_len_check CHECK (char_length(target_key) BETWEEN 1 AND 128),
    CONSTRAINT targets_name_len_check CHECK (char_length(target_name) BETWEEN 1 AND 200),
    CONSTRAINT targets_address_len_check CHECK (char_length(address) BETWEEN 1 AND 512),
    CONSTRAINT targets_interval_check CHECK (interval_ms > 0),
    CONSTRAINT targets_timeout_check CHECK (timeout_ms > 0),
    CONSTRAINT targets_failure_threshold_check CHECK (failure_threshold > 0),
    CONSTRAINT targets_recovery_threshold_check CHECK (recovery_threshold > 0),
    CONSTRAINT targets_generation_check CHECK (config_generation >= 1),
    CONSTRAINT targets_type_check CHECK (target_type IN ('HTTP','TCP','KUBERNETES')),
    CONSTRAINT targets_status_check CHECK (status IN ('UNKNOWN','NORMAL','ABNORMAL')),
    CONSTRAINT targets_consecutive_check CHECK (consecutive_failures >= 0 AND consecutive_successes >= 0)
);
CREATE UNIQUE INDEX service_monitor_targets_active_key
    ON registry.service_monitor_targets (service_id, target_key)
    WHERE deleted_at IS NULL;
CREATE INDEX targets_service_idx ON registry.service_monitor_targets (service_id) WHERE deleted_at IS NULL;
CREATE INDEX targets_enabled_idx ON registry.service_monitor_targets (enabled, deleted_at, last_check_at);

CREATE TABLE registry.service_source_states (
    service_id     uuid NOT NULL REFERENCES registry.services(service_id) ON DELETE RESTRICT,
    source_type    text NOT NULL,
    status         text NOT NULL DEFAULT 'UNKNOWN',
    reason         text NOT NULL DEFAULT '',
    updated_at     timestamptz NOT NULL DEFAULT now(),
    last_change_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (service_id, source_type),
    CONSTRAINT source_states_type_check CHECK (source_type IN ('MONITOR','APP')),
    CONSTRAINT source_states_status_check CHECK (status IN ('UNKNOWN','NORMAL','ABNORMAL'))
);

CREATE TABLE registry.outbox_events (
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
    ON registry.outbox_events (dedupe_key)
    WHERE dedupe_key IS NOT NULL;
CREATE INDEX outbox_pending_idx ON registry.outbox_events (next_retry_at, created_at) WHERE published_at IS NULL;
CREATE INDEX outbox_claim_idx ON registry.outbox_events (claim_until) WHERE published_at IS NULL;

CREATE TABLE registry.consumer_idempotency (
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
CREATE INDEX consumer_idem_processed_idx ON registry.consumer_idempotency (processed_at);