-- 0001: 服务主链四表（services / service_monitor_targets / service_source_states / episodes）
-- 纯 SQL：不含 BEGIN/COMMIT（runner 单文件单事务）；禁止 DROP/CREATE SCHEMA。

CREATE TABLE system.services (
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
CREATE INDEX services_enabled_idx ON system.services (enabled, deleted_at);

CREATE TABLE system.service_monitor_targets (
    target_id             uuid PRIMARY KEY,
    service_id            uuid NOT NULL REFERENCES system.services(service_id) ON DELETE RESTRICT,
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
    ON system.service_monitor_targets (service_id, target_key)
    WHERE deleted_at IS NULL;
CREATE INDEX targets_service_idx ON system.service_monitor_targets (service_id) WHERE deleted_at IS NULL;
CREATE INDEX targets_enabled_idx ON system.service_monitor_targets (enabled, deleted_at, last_check_at);

CREATE TABLE system.service_source_states (
    service_id     uuid NOT NULL REFERENCES system.services(service_id) ON DELETE RESTRICT,
    source_type    text NOT NULL,
    status         text NOT NULL DEFAULT 'UNKNOWN',
    reason         text NOT NULL DEFAULT '',
    updated_at     timestamptz NOT NULL DEFAULT now(),
    last_change_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (service_id, source_type),
    CONSTRAINT source_states_type_check CHECK (source_type IN ('MONITOR','APP')),
    CONSTRAINT source_states_status_check CHECK (status IN ('UNKNOWN','NORMAL','ABNORMAL'))
);

CREATE TABLE system.episodes (
    episode_id              uuid PRIMARY KEY,
    service_id              uuid NOT NULL REFERENCES system.services(service_id) ON DELETE RESTRICT,
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
CREATE UNIQUE INDEX episodes_one_active ON system.episodes (service_id) WHERE status = 'ACTIVE';
CREATE INDEX episodes_service_idx ON system.episodes (service_id, started_at DESC);
CREATE INDEX episodes_status_idx ON system.episodes (status, started_at DESC);