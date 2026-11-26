"""共享的应用端口：时钟与标识符生成。"""

from __future__ import annotations

from alert_agent.application.shared.ports import Clock, IdGen

__all__ = ["Clock", "IdGen"]