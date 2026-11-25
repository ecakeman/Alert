"""T18 static verification: DTO rules, token dep, registry, app wiring (temporary)."""

import asyncio
import sys

sys.path.insert(0, "/home/sancho/workspace/projects/Alert/services/agent-runtime/src")

from fastapi import FastAPI, HTTPException
from fastapi.testclient import TestClient

from alert_agent.bootstrap import wiring
from alert_agent.bootstrap.app import create_app
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
from alert_agent.transport.http.deps import require_internal_token
from alert_agent.transport.http.dto.chat import ChatRequestDto
from alert_agent.transport.http.dto.search import SearchRequestDto, SearchResponseDto
from alert_agent.transport.http.mapper import chunk_to_hit, to_search_params
from alert_agent.domain.retrieval.chunk import RetrievedChunk


def build_settings(token: str = "real-internal-token") -> Settings:
    return Settings(
        shared=SharedSettings(database_url_cognitive="postgresql://u:p@h:5432/db"),
        gateway=GatewaySettings(
            agent_grpc_target="localhost:9090",
            agent_grpc_listen="0.0.0.0:9091",
            http_listen="0.0.0.0:8081",
            internal_token=token,
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
    # --- DTO validation rules ---
    req = SearchRequestDto.model_validate({"query": "q", "docIds": ["d1"], "topK": 8})
    assert req.doc_ids == ["d1"] and req.top_k == 8
    try:
        SearchRequestDto(query="q", topK=21)
        raise SystemExit("topK>20 must fail (7.4.4)")
    except Exception:
        pass

    chat = ChatRequestDto.model_validate({"sessionId": "s", "message": "m"})
    assert chat.session_id == "s" and chat.top_k == 5
    for bad in (
        {"sessionId": "s", "message": "m", "history": [{"role": "system", "content": "x"}]},
        {"sessionId": "s", "message": "m", "history": [{"role": "user", "content": "x"}] * 7},
        {"sessionId": "s", "message": ""},
    ):
        try:
            ChatRequestDto.model_validate(bad)
            raise SystemExit(f"must reject: {bad}")
        except Exception:
            pass

    # --- mapper: the single conversion point (CR-LAY-10) ---
    params = to_search_params(SearchRequestDto(query="q", docIds=["d1"], topK=7))
    assert params == {"query": "q", "doc_ids": ("d1",), "vector_top_k": 7, "fusion_top_k": 7}
    chunk = RetrievedChunk(
        chunk_id="c1", doc_id="d1", content="body", score=0.9, leg="vector"
    )
    hit = chunk_to_hit(chunk).model_dump(by_alias=True)
    assert hit["chunkId"] == "c1" and hit["score"] == 0.9

    # --- app wiring: routes registered, token enforced ---
    settings = build_settings()
    app = create_app(settings)

    # searcher not wired until lifespan startup: /search returns 503 NOT_READY
    # (a 503 itself proves the route is registered; an unregistered route
    # would yield 404)
    client = TestClient(app, raise_server_exceptions=False)
    response = client.post(
        "/search",
        json={"query": "q"},
        headers={"X-Internal-Token": "real-internal-token"},
    )
    assert response.status_code == 503 and response.json()["detail"]["code"] == "NOT_READY", response.text

    # missing token -> 401 (constant-time compare path)
    response = client.post("/search", json={"query": "q"})
    assert response.status_code == 401 and response.json()["detail"]["code"] == "UNAUTHORIZED"
    response = client.post(
        "/search", json={"query": "q"}, headers={"X-Internal-Token": "wrong"}
    )
    assert response.status_code == 401

    # wiring exposes build_chat_answer & build_retrieval_searcher
    assert callable(wiring.build_chat_answer)
    assert callable(wiring.build_retrieval_searcher)

    print("T18 static verification OK")


main_sync()