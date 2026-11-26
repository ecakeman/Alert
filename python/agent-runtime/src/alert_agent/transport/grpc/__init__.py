"""gRPC 传输层：AgentTaskService servicer（薄协议适配，CR-GW-02/03）。"""

from __future__ import annotations

from alert_agent.transport.grpc.servicer import AgentTaskServicer

__all__ = ["AgentTaskServicer"]