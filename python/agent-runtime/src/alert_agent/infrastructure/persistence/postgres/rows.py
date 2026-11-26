"""Postgres 持久化的行模型（仅基础设施层使用，CR-LAY-07）。

行结构只存放在此，绝不泄漏到应用/领域类型；仓储在边界处把行映射为
端口模型。
"""

from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime


@dataclass(frozen=True, slots=True)
class ReceiptRow:
    """回执仓储使用的 cognitive.agent_task_receipts 投影。"""

    task_id: str
    generation: int
    task_type: str
    episode_id: str
    service_id: str
    thread_id: str
    status: str
    lease_token: str | None = None
    input_version_from: int | None = None
    input_version_to: int | None = None
    monitor_watermark: int | None = None
    app_event_watermark: int | None = None
    deadline_at: datetime | None = None