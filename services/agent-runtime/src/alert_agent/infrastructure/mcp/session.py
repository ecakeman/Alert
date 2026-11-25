"""MCP Streamable HTTP session client (T21, design 4.4, CR-GW-05/06/07,
CR-REG-35).

Singleton long-lived session (CR-GW-06): initialize (capability negotiation,
peer protocol version kept) -> initialized notification -> tools/list with a
TTL cache -> tools/call reuses the session; never re-initializes per call.
Streamable-HTTP discipline (CR-REG-35): dual Accept header, ``Mcp-Session-Id``
response header stored and replayed, SSE responses parsed. Call discipline
(CR-GW-07): per-call timeout derived from the single gateway RPC setting,
JSON-RPC id validation, bounded retries on transport failures, and
``McpToolError`` (code/message/data) for ``isError`` results and JSON-RPC
errors -- never an empty-string result. The tools directory is returned
without any name filtering (C6 lesson).
"""

from __future__ import annotations

import asyncio
import itertools
import json
import logging
import time

import httpx

from alert_agent.application.mcp.ports import McpToolInfo
from alert_agent.domain.shared import ErrorCode

LOGGER = logging.getLogger(__name__)

_PROTOCOL_VERSION = "2025-06-18"
_TOOLS_TTL_SECONDS = 60.0


class McpToolError(Exception):
    """Structured MCP failure (CR-GW-07): code, message and optional data."""

    def __init__(self, code: str, message: str, data: object = None) -> None:
        super().__init__(f"{code}: {message}")
        self.code = code
        self.message = message
        self.data = data


class McpSessionManager:
    """``McpSession`` port implementation (T10 contract, T21 transport)."""

    def __init__(
        self,
        *,
        endpoint: str,
        token: str,
        timeout_seconds: float,
        max_retries: int = 2,
        backoff_seconds: float = 0.5,
        transport: httpx.AsyncBaseTransport | None = None,
    ) -> None:
        self._endpoint = endpoint.rstrip("/")
        self._token = token
        self._timeout = httpx.Timeout(
            connect=timeout_seconds / 2.0,
            read=timeout_seconds,
            write=timeout_seconds,
            pool=timeout_seconds / 2.0,
        )
        self._max_retries = max_retries
        self._backoff = backoff_seconds
        self._transport = transport  # test seam; None uses the default stack
        self._session_id: str | None = None
        self._initialized = False
        self._init_lock = asyncio.Lock()
        self._id_counter = itertools.count(1)
        self._tools: list[McpToolInfo] = []
        self._tools_cached_at: float = 0.0

    async def list_tools(self) -> list[McpToolInfo]:
        if time.monotonic() - self._tools_cached_at < _TOOLS_TTL_SECONDS and self._tools:
            return self._tools
        await self._ensure_session()
        payload = await self._rpc("tools/list", {})
        tools = payload.get("tools")
        if not isinstance(tools, list):
            raise McpToolError(
                ErrorCode.MCP_TOOL_ERROR.value, "tools/list response has no tools array"
            )
        self._tools = [
            McpToolInfo(
                name=str(entry.get("name", "")),
                description=str(entry.get("description", "")),
            )
            for entry in tools
            if isinstance(entry, dict) and entry.get("name")
        ]
        self._tools_cached_at = time.monotonic()
        return self._tools

    async def call_tool(self, name: str, arguments: dict) -> list[dict]:
        await self._ensure_session()
        payload = await self._rpc("tools/call", {"name": name, "arguments": arguments})
        if payload.get("isError") is True:
            content = payload.get("content") or []
            text = next(
                (
                    entry.get("text", "")
                    for entry in content
                    if isinstance(entry, dict)
                ),
                "",
            )
            raise McpToolError(
                ErrorCode.MCP_TOOL_ERROR.value,
                text or f"tool {name} reported isError",
                data=content,
            )
        content = payload.get("content")
        if not isinstance(content, list):
            raise McpToolError(
                ErrorCode.MCP_TOOL_ERROR.value,
                f"tool {name} returned no content blocks",
            )
        return [entry for entry in content if isinstance(entry, dict)]

    async def _ensure_session(self) -> None:
        if self._initialized:
            return
        async with self._init_lock:
            if self._initialized:
                return
            await self._initialize()
            self._initialized = True

    async def _initialize(self) -> None:
        payload = await self._rpc(
            "initialize",
            {
                "protocolVersion": _PROTOCOL_VERSION,
                "capabilities": {},
                "clientInfo": {"name": "alert-agent-runtime", "version": "0.1.0"},
            },
        )
        peer_version = payload.get("protocolVersion")
        if peer_version:
            LOGGER.info("mcp peer protocol version: %s", peer_version)
        # notifications/initialized: no id, no response expected.
        await self._rpc("notifications/initialized", {}, notify=True)

    async def _rpc(
        self, method: str, params: dict, *, notify: bool = False
    ) -> dict:
        request_id = next(self._id_counter)
        body = {"jsonrpc": "2.0", "method": method, "params": params}
        if not notify:
            body["id"] = request_id
        headers = {
            "Accept": "application/json, text/event-stream",
            "Content-Type": "application/json",
            "Authorization": f"Bearer {self._token}",
        }
        if self._session_id:
            headers["Mcp-Session-Id"] = self._session_id
        last_error: Exception | None = None
        for attempt in range(self._max_retries + 1):
            try:
                async with httpx.AsyncClient(timeout=self._timeout, transport=self._transport) as client:
                    response = await client.post(self._endpoint, json=body, headers=headers)
            except httpx.HTTPError as exc:
                last_error = exc
                LOGGER.warning("mcp %s transport failure: %s", method, type(exc).__name__)
                if attempt < self._max_retries:
                    await asyncio.sleep(self._backoff * (attempt + 1))
                continue
            session_id = response.headers.get("Mcp-Session-Id")
            if session_id:
                self._session_id = session_id
            if response.status_code >= 400:
                raise McpToolError(
                    ErrorCode.MCP_TOOL_ERROR.value,
                    f"mcp endpoint returned {response.status_code} for {method}",
                )
            content_type = response.headers.get("Content-Type", "")
            if "text/event-stream" in content_type:
                return self._parse_sse(response.text, request_id, notify)
            if notify:
                # Notifications carry no response body (e.g. HTTP 202).
                return {}
            data = response.json()
            return self._unwrap(data, request_id, notify, method)
        raise McpToolError(
            ErrorCode.MCP_TOOL_ERROR.value,
            f"mcp {method} failed after {self._max_retries + 1} attempts: {type(last_error).__name__}",
        )

    def _parse_sse(self, text: str, request_id: int, notify: bool, method: str = "") -> dict:
        """Streamable-HTTP SSE response: yield the first matching JSON-RPC
        response for our id (or the first data frame for notifications)."""
        for line in text.splitlines():
            line = line.strip()
            if not line.startswith("data:"):
                continue
            data = line[len("data:") :].strip()
            if not data or data == "[DONE]":
                continue
            try:
                message = json.loads(data)
            except ValueError:
                continue
            if notify:
                return {}
            if message.get("id") == request_id:
                return self._unwrap(message, request_id, notify, method)
        raise McpToolError(
            ErrorCode.MCP_TOOL_ERROR.value,
            f"mcp SSE response carried no reply for id {request_id}",
        )

    def _unwrap(self, message: dict, request_id: int, notify: bool, method: str) -> dict:
        if notify:
            return {}
        if message.get("id") != request_id:
            raise McpToolError(
                ErrorCode.MCP_TOOL_ERROR.value,
                f"mcp response id mismatch (expected {request_id})",
            )
        error = message.get("error")
        if error:
            raise McpToolError(
                str(error.get("code", ErrorCode.MCP_TOOL_ERROR.value)),
                str(error.get("message", "mcp json-rpc error")),
                data=error.get("data"),
            )
        result = message.get("result")
        if not isinstance(result, dict):
            raise McpToolError(
                ErrorCode.MCP_TOOL_ERROR.value,
                f"mcp {method} returned no result object",
            )
        return result