-- 0001: monitor 本域基线（service_monitor_points / monitor_target_projection / outbox / idempotency）
-- service_id / target_id 为逻辑引用（跨域 FK 已按 design 2.3.2 删除），其余 CHECK/索引逐条保留。
-- 纯 SQL：不含 BEGIN/COMMIT；禁止 DROP/CREATE SCHEMA。

CREATE TABLE monitor.service_monitor_points (
    point_seq        bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    service_id       uuid NOT NULL,
    target_id        uuid NOT NULL,
    target_key       text NOT NULL,
    target_type      text NOT NULL,
    observed_at      timestamptz NOT NULL,
    received_at      timestamptz NOT NULL DEFAULT now(),
    check_status     text NOT NULL,
    status_class     text NOT NULL DEFAULT '',
    error_class      text NOT NULL DEFAULT '',
    state_fingerprint text NOT NULL DEFAULT '',
    latency_ms       integer,
    probe_generation bigint NOT NULL,
    CONSTRAINT monitor_points_status_check CHECK (check_status IN ('SUCCESS','FAILURE','TIMEOUT','ERROR')),
    CONSTRAINT monitor_points_latency_check CHECK (latency_ms IS NULL OR latency_ms >= 0),
    CONSTRAINT monitor_points_generation_check CHECK (probe_generation >= 1)
);
CREATE INDEX monitor_points_service_idx ON monitor.service_monitor_points (service_id, point_seq DESC);
CREATE INDEX monitor_points_target_idx ON monitor.service_monitor_points (target_id, observed_at DESC);
CREATE INDEX monitor_points_observed_idx ON monitor.service_monitor_points (observed_at);

-- CQRS 读模型：调度所需的目标配置 + 自产的 last_probe_at。
-- 不复制 status/consecutive_*（探测结果回传 registry，不在投影保存）。
CREATE TABLE monitor.monitor_target_projection (
    target_id                uuid PRIMARY KEY,
    service_id               uuid NOT NULL,
    target_key               text NOT NULL,
    target_type              text NOT NULL DEFAULT 'HTTP',
    address                  text NOT NULL,
    interval_ms              integer NOT NULL,
    timeout_ms               integer NOT NULL,
    failure_threshold        integer NOT NULL DEFAULT 3,
    recovery_threshold       integer NOT NULL DEFAULT 2,
    is_required              boolean NOT NULL DEFAULT true,
    enabled                  boolean NOT NULL DEFAULT true,
    config_generation        bigint NOT NULL DEFAULT 1,
    monitor_aggregation_mode text NOT NULL DEFAULT 'ALL_REQUIRED',
    last_probe_at            timestamptz,
    deleted_at               timestamptz,
    synced_at                timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT projection_target_type_check CHECK (target_type IN ('HTTP','TCP','KUBERNETES')),
    CONSTRAINT projection_interval_check CHECK (interval_ms > 0),
    CONSTRAINT projection_timeout_check CHECK (timeout_ms > 0),
    CONSTRAINT projection_failure_threshold_check CHECK (failure_threshold > 0),
    CONSTRAINT projection_recovery_threshold_check CHECK (recovery_threshold > 0),
    CONSTRAINT projection_generation_check CHECK (config_generation >= 1),
    CONSTRAINT projection_aggregation_check CHECK (monitor_aggregation_mode IN ('ALL_REQUIRED','ANY_REQUIRED','MAJORITY'))
);
CREATE INDEX projection_service_idx ON monitor.monitor_target_projection (service_id) WHERE deleted_at IS NULL;
CREATE INDEX projection_due_idx ON monitor.monitor_target_projection (enabled, deleted_at, last_probe_at);

CREATE TABLE monitor.outbox_events (
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
    ON monitor.outbox_events (dedupe_key)
    WHERE dedupe_key IS NOT NULL;
CREATE INDEX outbox_pending_idx ON monitor.outbox_events (next_retry_at, created_at) WHERE published_at IS NULL;
CREATE INDEX outbox_claim_idx ON monitor.outbox_events (claim_until) WHERE published_at IS NULL;

CREATE TABLE monitor.consumer_idempotency (
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
CREATE INDEX consumer_idem_processed_idx ON monitor.consumer_idempotency (processed_at);