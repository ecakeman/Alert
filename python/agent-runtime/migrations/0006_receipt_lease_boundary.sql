-- 0006: 持久化租约凭据与冻结边界（P5.9 GAP-1）
-- 受理时把 SubmitTask 的 lease_token 与 5 项 FrozenBoundary 落库，使进程
-- 重启后能重建 TaskSubmission 并复用正常执行路径；不回改 0001。
-- 全部列可空、无默认：迁移前的老回执保持 NULL，诚实表示"不可续租"。
-- 不新增索引：复用 agent_task_receipts_unfinished_idx 的 status 部分索引。

ALTER TABLE cognitive.agent_task_receipts
    ADD COLUMN IF NOT EXISTS lease_token text,
    ADD COLUMN IF NOT EXISTS input_version_from bigint,
    ADD COLUMN IF NOT EXISTS input_version_to bigint,
    ADD COLUMN IF NOT EXISTS monitor_watermark bigint,
    ADD COLUMN IF NOT EXISTS app_event_watermark bigint,
    ADD COLUMN IF NOT EXISTS deadline_at timestamptz;