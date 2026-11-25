"""MCP 会话端口（design 4.4，CR-GW-05/06/07）。

streamable-HTTP 会话客户端（T21）实现此端口；诊断工具执行器（T13）只依赖
该端口，因此不会有 HTTP 客户端泄漏进应用层（CR-GW-05）。调用方在执行前
先用 ``list_tools`` 校验 ``CallMcpToolAction.name``（CR-DIAG-06）。
"""

from __future__ import annotations

from typing import Any, Protocol

from pydantic import BaseModel, ConfigDict, Field


class McpToolInfo(BaseModel):
    """实时 tools/list 目录中的一个条目（无名称过滤）。"""

    model_config = ConfigDict(frozen=True)

    name: str = Field(min_length=1)
    description: str = ""


class McpSession(Protocol):
    """长生命周期会话：initialize/initialized 一次完成，tools/list 按 TTL
    缓存，tools/call 复用会话（CR-GW-06）；单次调用超时来自单一配置源
    （CR-REG-32）。"""

    async def list_tools(self) -> list[McpToolInfo]: ...

    async def call_tool(
        self, name: str, arguments: dict[str, Any]
    ) -> list[dict[str, Any]]: ...