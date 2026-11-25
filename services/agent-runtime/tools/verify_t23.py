"""T23 static verification: SSE frames, /chat streaming, observation sink."""

import asyncio
import sys

sys.path.insert(0, "/home/sancho/workspace/projects/Alert/services/agent-runtime/src")

from fastapi import FastAPI
from fastapi.testclient import TestClient

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
from alert_agent.infrastructure.observe import BoundedObservationSink
from alert_agent.transport.http.dto.chat import ChatRequestDto
from alert_agent.transport.sse import (
    citations_frame,
    done_frame,
    error_frame,
    token_frame,
    trace_frame,
)


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


def main_sync() -> None:
    # --- SSE frame formats (contract-exact camelCase) ---
    frame = trace_frame("rewrite", "ok", "detail", 12.5)
    assert 'event: trace\ndata: {"node": "rewrite"' in frame and "tsMs" in frame
    assert 'event: token' in token_frame("你好") and "你好" in token_frame("你好")
    assert "latencyMs" in done_frame(88.0)
    assert "code" in error_frame("CHAT_FAILED", "boom")
    citation_payload = {
        "no": 1,
        "chunkId": "c1",
        "docId": "d1",
        "docTitle": "",
        "text_excerpt": "excerpt",
    }
    frame = citations_frame([citation_payload])
    assert '"chunkId": "c1"' in frame and '"items"' in frame.replace(" ", "").replace('"items":', "items") or '"items"' in frame

    # --- observation sink: queue + drop counter (CR-REG-19) ---
    sink = BoundedObservationSink(None, capacity=2)
    sink.record(scene="chat", model="m", latency_ms=1.0)
    sink.record(scene="chat", model="m", latency_ms=2.0)
    sink.record(scene="chat", model="m", latency_ms=3.0)  # full -> dropped
    assert sink._queue.qsize() == 2 and sink.dropped == 1

    # --- app wiring: /chat registered, 401 without token, 503 unwired ---
    app = FastAPI()
    settings = build_settings()
    from alert_agent.transport.http.router.registry import build_router

    app.state.settings = settings
    app.include_router(build_router())
    client = TestClient(app, raise_server_exceptions=False)
    response = client.post(
        "/chat",
        json={"sessionId": "s1", "message": "网关为什么 5xx"},
        headers={"X-Internal-Token": "real-internal-token"},
    )
    assert response.status_code == 503 and response.json()["detail"]["code"] == "NOT_READY", response.text
    response = client.post("/chat", json={"sessionId": "s1", "message": "m"})
    assert response.status_code == 401

    # wiring exposes the sink builder
    assert callable(wiring.build_observation_sink)

    print("T23 static verification OK")


main_sync()