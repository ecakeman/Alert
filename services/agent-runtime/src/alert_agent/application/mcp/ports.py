"""MCP session port (design 4.4, CR-GW-05/06/07).

The streamable-HTTP session client (T21) implements this port; the diagnosis
tool executor (T13) only depends on the port, so no HTTP client leaks into the
application layer (CR-GW-05). Callers check ``list_tools`` against
``CallMcpToolAction.name`` before executing (CR-DIAG-06).
"""

from __future__ import annotations

from typing import Any, Protocol

from pydantic import BaseModel, ConfigDict, Field


class McpToolInfo(BaseModel):
    """One entry of the live tools/list directory (no name filtering)."""

    model_config = ConfigDict(frozen=True)

    name: str = Field(min_length=1)
    description: str = ""


class McpSession(Protocol):
    """Long-lived session: initialize/initialized once, tools/list cached with
    TTL, tools/call reuses the session (CR-GW-06); per-call timeout derives
    from the single configuration source (CR-REG-32)."""

    async def list_tools(self) -> list[McpToolInfo]: ...

    async def call_tool(
        self, name: str, arguments: dict[str, Any]
    ) -> list[dict[str, Any]]: ...