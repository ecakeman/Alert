"""T22 static verification: agent card, message/send mapping, knowledge client."""

import asyncio
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
from alert_agent.domain.knowledge.models import InvocationV1
from alert_agent.domain.shared import ErrorCode, FatalError, RetryableError
from alert_agent.infrastructure.a2a import build_agent_card
from alert_agent.infrastructure.knowledge import A2aKnowledgeClient


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
            knowledge_a2a_endpoint="http://knowledge.local/a2a",
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


class FakeKnowledgeServer:
    """Mock knowledge agent with programmable responses."""

    def __init__(self) -> None:
        self.mode = "ok"  # ok | not_completed | bad_schema | reject

    def handler(self, request: httpx.Request) -> httpx.Response:
        if self.mode == "reject":
            return httpx.Response(403, json={"detail": "forbidden"})
        if self.mode == "not_completed":
            return httpx.Response(
                200,
                json={"status": "FAILED", "error_code": "K1", "safe_message": "indexer down"},
            )
        if self.mode == "bad_schema":
            return httpx.Response(
                200,
                json={"status": "COMPLETED", "result": {"results": [{"nonsense": 1}]}},
            )
        return httpx.Response(
            200,
            json={
                "status": "COMPLETED",
                "result": {
                    "results": [
                        {"hitId": "h1", "title": "知识条目", "content": "内容正文", "score": 0.8}
                    ]
                },
            },
        )


async def main() -> None:
    # --- agent card (three skills) ---
    card = build_agent_card()
    assert {skill.id for skill in card.skills} == {"diagnosis", "archive", "retrieval"}

    # --- knowledge client: COMPLETED path with validation (CR-KNOW-07) ---
    server = FakeKnowledgeServer()
    settings = build_settings()
    client = A2aKnowledgeClient(
        endpoint=settings.diagnosis.knowledge_a2a_endpoint,
        token=settings.gateway.internal_token,
        timeout_seconds=5.0,
        transport=httpx.MockTransport(server.handler),
    )
    invocation = InvocationV1(
        invocation_id="t1-2-abc",
        skill="qa",
        query="db down",
        context={"knownFacts": ["已知事实"], "episodeId": "e1"},
    )
    hits = await client.invoke(invocation, traceparent="00-trace-span-01")
    assert len(hits) == 1 and hits[0].hit_id == "h1"

    # knownFacts serialization is JSON text with ensure_ascii=False (CR-KNOW-02)
    payload = invocation.model_dump(by_alias=True)
    import json as jsonlib

    assert "已知事实" in jsonlib.dumps(payload["context"]["knownFacts"], ensure_ascii=False)

    # --- status != COMPLETED -> diagnosable error (CR-KNOW-08) ---
    server.mode = "not_completed"
    try:
        await client.invoke(invocation)
        raise SystemExit("non-completed status must fail")
    except FatalError as exc:
        assert exc.code == ErrorCode.KNOWLEDGE_INVOCATION_FAILED
        assert "indexer down" in exc.message

    # --- schema mismatch -> KNOWLEDGE_SCHEMA_ERROR ---
    server.mode = "bad_schema"
    try:
        await client.invoke(invocation)
        raise SystemExit("schema mismatch must fail")
    except FatalError as exc:
        assert exc.code == ErrorCode.KNOWLEDGE_SCHEMA_ERROR

    # --- deterministic rejection: no retry, immediate FatalError ---
    server.mode = "reject"
    try:
        await client.invoke(invocation)
        raise SystemExit("403 must fail without retry")
    except FatalError:
        pass

    # --- wiring: knowledge client assembled and wired into diagnosis executor ---
    import grpc.aio

    knowledge = wiring.build_knowledge_client(settings)
    assert isinstance(knowledge, A2aKnowledgeClient)
    channel = grpc.aio.insecure_channel("localhost:9090")
    executor = wiring.build_diagnosis_executor(settings, None, channel)
    assert isinstance(executor._tool_executor._knowledge, A2aKnowledgeClient)

    print("T22 static verification OK")


asyncio.run(main())