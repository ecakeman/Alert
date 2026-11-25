"""gRPC transport: AgentTaskService servicer (thin protocol adaptation, CR-GW-02/03)."""

from __future__ import annotations

from alert_agent.transport.grpc.servicer import AgentTaskServicer

__all__ = ["AgentTaskServicer"]