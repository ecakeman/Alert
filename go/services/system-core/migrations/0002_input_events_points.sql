-- 0002: 输入事实与水位线三表（episode_input_versions / service_monitor_points / episode_app_events）
-- 纯 SQL：不含 BEGIN/COMMIT；禁止 DROP。

CREATE TABLE system.episode_input_versions (
    episode_id          uuid NOT NULL REFERENCES system.episodes(episode_id) ON DELETE RESTRICT,
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

CREATE TABLE system.service_monitor_points (
    point_seq        bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    service_id       uuid NOT NULL REFERENCES system.services(service_id) ON DELETE RESTRICT,
    target_id        uuid NOT NULL REFERENCES system.service_monitor_targets(target_id) ON DELETE RESTRICT,
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
CREATE INDEX monitor_points_service_idx ON system.service_monitor_points (service_id, point_seq DESC);
CREATE INDEX monitor_points_target_idx ON system.service_monitor_points (target_id, observed_at DESC);
CREATE INDEX monitor_points_observed_idx ON system.service_monitor_points (observed_at);

CREATE TABLE system.episode_app_events (
    event_record_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    app_event_seq   bigint NOT NULL,
    service_id      uuid NOT NULL REFERENCES system.services(service_id) ON DELETE RESTRICT,
    episode_id      uuid REFERENCES system.episodes(episode_id) ON DELETE RESTRICT,
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
CREATE INDEX app_events_service_idx ON system.episode_app_events (service_id, app_event_seq DESC);
CREATE INDEX app_events_episode_idx ON system.episode_app_events (episode_id);