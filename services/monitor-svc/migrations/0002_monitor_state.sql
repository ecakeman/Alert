-- 0002: monitor 自产探测状态（target 级计数 + service 级聚合状态）
-- 说明：monitor_target_projection 只保存 registry 配置与自产 last_probe_at；
-- 探测计算出的状态/连续计数属 monitor 自产结果，落在这两张表，再以事件回传 registry。

CREATE TABLE monitor.monitor_target_state (
    target_id             uuid PRIMARY KEY,
    service_id            uuid NOT NULL,
    status                text NOT NULL DEFAULT 'UNKNOWN',
    consecutive_failures  integer NOT NULL DEFAULT 0,
    consecutive_successes integer NOT NULL DEFAULT 0,
    last_check_at         timestamptz,
    updated_at            timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT target_state_status_check CHECK (status IN ('UNKNOWN','NORMAL','ABNORMAL')),
    CONSTRAINT target_state_consecutive_check CHECK (consecutive_failures >= 0 AND consecutive_successes >= 0)
);
CREATE INDEX target_state_service_idx ON monitor.monitor_target_state (service_id);

CREATE TABLE monitor.monitor_source_state (
    service_id       uuid PRIMARY KEY,
    status           text NOT NULL DEFAULT 'UNKNOWN',
    aggregation_mode text NOT NULL DEFAULT 'ALL_REQUIRED',
    updated_at       timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT source_state_status_check CHECK (status IN ('UNKNOWN','NORMAL','ABNORMAL')),
    CONSTRAINT source_state_aggregation_check CHECK (aggregation_mode IN ('ALL_REQUIRED','ANY_REQUIRED','MAJORITY'))
);