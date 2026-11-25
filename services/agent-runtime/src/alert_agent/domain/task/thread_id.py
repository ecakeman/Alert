"""The single thread-id rule shared by receipt locking and recovery (CR-REG-43, Q4).

Same-episode diagnosis and archive tasks use one unified prefix so lock keys
never mismatch: ``diagnosis:{episode_id}`` / ``archive:{episode_id}`` (data
constraints 7.1.8). Receipt locking and the recovery scan (CR-TASK-10) must
both go through this function.
"""

from __future__ import annotations

_PREFIXES = {"DIAGNOSIS": "diagnosis", "ARCHIVE": "archive"}


def thread_id_of(task_type: str, episode_id: str) -> str:
    """Derive the unified thread id for one task type on one episode."""
    prefix = _PREFIXES.get(task_type)
    if prefix is None or not episode_id:
        raise ValueError("task_type must be DIAGNOSIS or ARCHIVE and episode_id must be non-empty")
    return f"{prefix}:{episode_id}"