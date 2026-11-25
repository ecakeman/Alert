"""T21 static verification: MCP session lifecycle, errors, no filtering."""

import asyncio
import json
import sys

sys.path.insert(0, "/home/sancho/workspace/projects/Alert/services/agent-runtime/src")

import httpx

from alert_agent.bootstrap import wiring
from alert_agent.config.settings import (
    DiagnosisSettings,
    GatewaySettings,
    IngestionSettings,
    LlmSettings,
    RetrievalSettings,
    Settings,
    SharedSettings,
    StorageSettings,
)
from alert_agent.infrastructure.mcp import McpSessionManager, McpToolError


def build_settings() -> Settings:
    return Settings(
        shared=SharedSettings(database_url_cognitive="postgresql://u:p@h:5432/db"),
        gateway=GatewaySettings(
            agent_grpc_target="localhost:9090",
            agent_grpc_listen="0.0.0.0:9091",
            http_listen="0.0.0.0:8081",
            internal_token="real-internal-token",
        ),
        diagnosis=DiagnosisSettings(
            mcp_endpoint="http://mcp.local",
            mcp_token="real-mcp-token",
            knowledge_a2a_endpoint="http://knowledge.local",
        ),
        retrieval=RetrievalSettings(),
        ingestion=IngestionSettings(ingest_file_root="/tmp/ingest"),
        llm=LlmSettings(
            llm_api_base="http://llm.local/v1",
            llm_api_key="real-key",
            llm_model="m",
            enable_embedding=False,
            enable_rerank=False,
        ),
        storage=StorageSettings(),
    )


class FakeMcpServer:
    """Minimal streamable-HTTP MCP endpoint over httpx.MockTransport."""

    def __init__(self) -> None:
        self.session_id = "sess-123"
        self.initialized = False
        self.tool_calls = 0
        self.tool_error = False

    def handler(self, request: httpx.Request) -> httpx.Response:
        body = json.loads(request.content.decode("utf-8"))
        method = body.get("method", "")
        request_id = body.get("id")
        headers = {
            "Mcp-Session-Id": self.session_id,
            "Content-Type": "application/json",
        }
        if method == "initialize":
            return httpx.Response(
                200,
                headers=headers,
                json={"jsonrpc": "2.0", "id": request_id, "result": {"protocolVersion": "2025-06-18"}},
            )
        if method == "notifications/initialized":
            self.initialized = True
            return httpx.Response(202, headers=headers)
        if method == "tools/list":
            # Includes a dotted and an undotted tool: NO name filtering (C6).
            tools = [
                {"name": "log.query", "description": "query logs"},
                {"name": "ping", "description": "plain ping"},
            ]
            return httpx.Response(
                200,
                headers=headers,
                json={"jsonrpc": "2.0", "id": request_id, "result": {"tools": tools}},
            )
        if method == "tools/call":
            self.tool_calls += 1
            if self.tool_error:
                return httpx.Response(
                    200,
                    headers=headers,
                    json={
                        "jsonrpc": "2.0",
                        "id": request_id,
                        "result": {"isError": True, "content": [{"type": "text", "text": "boom"}]},
                    },
                )
            return httpx.Response(
                200,
                headers=headers,
                json={
                    "jsonrpc": "2.0",
                    "id": request_id,
                    "result": {"content": [{"type": "text", "text": "ok"}]},
                },
            )
        return httpx.Response(
            200,
            headers=headers,
            json={"jsonrpc": "2.0", "id": request_id, "error": {"code": -32601, "message": "method not found"}},
        )


async def main() -> None:
    server = FakeMcpServer()
    transport = httpx.MockTransport(server.handler)
    manager = McpSessionManager(
        endpoint="http://mcp.local/",
        token="real-mcp-token",
        timeout_seconds=5.0,
        transport=transport,
    )

    # first list_tools triggers initialize + initialized + tools/list
    tools = await manager.list_tools()
    assert server.initialized is True
    assert manager._session_id == "sess-123"
    names = {tool.name for tool in tools}
    assert names == {"log.query", "ping"}, names  # no name filtering (C6)

    # TTL cache: second call does not hit the wire
    calls_before = sum(1 for _ in ())
    tools_again = await manager.list_tools()
    assert tools_again == tools

    # tools/call returns the full content list
    content = await manager.call_tool("log.query", {"q": "5xx"})
    assert content == [{"type": "text", "text": "ok"}]

    # isError -> structured McpToolError (CR-GW-07)
    server.tool_error = True
    try:
        await manager.call_tool("log.query", {})
        raise SystemExit("isError must raise McpToolError")
    except McpToolError as exc:
        assert exc.message == "boom" and exc.data

    # json-rpc error -> McpToolError with the peer code
    try:
        await manager._rpc("no_such_method", {})
        raise SystemExit("json-rpc error must raise")
    except McpToolError as exc:
        assert exc.code == "-32601"

    # SSE response parsing
    sse_body = (
        'data: {"jsonrpc": "2.0", "id": 99, "result": {"content": [{"type": "text", "text": "sse"}]}}\n\n'
    )
    parsed = manager._parse_sse(sse_body, 99, notify=False, method="tools/call")
    assert parsed["content"][0]["text"] == "sse"

    # wiring: diagnosis executor gets the MCP session by default
    import grpc.aio

    settings = build_settings()
    channel = grpc.aio.insecure_channel("localhost:9090")  # lazy, no dial
    executor = wiring.build_diagnosis_executor(settings, None, channel)
    tool_executor = executor._tool_executor
    assert tool_executor._mcp is not None
    assert isinstance(tool_executor._mcp, McpSessionManager)

    print("T21 static verification OK")


asyncio.run(main())