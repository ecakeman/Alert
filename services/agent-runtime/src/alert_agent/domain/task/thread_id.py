"""由回执锁定与恢复共享的唯一 thread-id 规则（CR-REG-43，Q4）。

同 episode 的诊断与归档任务使用统一前缀，因此锁 key
绝不失配：``diagnosis:{episode_id}`` / ``archive:{episode_id}``（数据
约束 7.1.8）。回执锁定与恢复扫描（CR-TASK-10）都必须
经过此函数。
"""

from __future__ import annotations

_PREFIXES = {"DIAGNOSIS": "diagnosis", "ARCHIVE": "archive"}


def thread_id_of(task_type: str, episode_id: str) -> str:
    """为某个 episode 上的一种任务类型推导统一的 thread id。"""
    prefix = _PREFIXES.get(task_type)
    if prefix is None or not episode_id:
        raise ValueError("task_type must be DIAGNOSIS or ARCHIVE and episode_id must be non-empty")
    return f"{prefix}:{episode_id}"