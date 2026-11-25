"""Row models for Postgres persistence (infrastructure-only, CR-LAY-07).

Row shapes live here and never leak into application/domain types; the
repositories map rows to port models at the boundary.
"""

from __future__ import annotations

from dataclasses import dataclass


@dataclass(frozen=True, slots=True)
class ReceiptRow:
    """Projection of cognitive.agent_task_receipts used by the receipt repo."""

    task_id: str
    generation: int
    task_type: str
    episode_id: str
    service_id: str
    thread_id: str
    status: str