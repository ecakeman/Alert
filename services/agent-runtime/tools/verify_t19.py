"""T19 static verification: safety rules, idempotent acceptance, app wiring."""

import asyncio
import sys
from pathlib import Path

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
from alert_agent.domain.ingestion import (
    PathRuleError,
    resolve_within_root,
    validate_callback_url,
)
from alert_agent.transport.http.dto.ingest import IngestRequestDto
from alert_agent.transport.http.mapper import to_accept_kwargs


class FakeStore:
    def __init__(self) -> None:
        self.jobs: dict[str, object] = {}

    async def create(self, job):
        if job.doc_id in self.jobs:
            return False
        self.jobs[job.doc_id] = job
        return True

    async def get(self, doc_id):
        return self.jobs.get(doc_id)

    async def mark(self, doc_id, status, *, chunk_count=None, error_msg=""):
        if doc_id not in self.jobs:
            return False
        return True

    async def list_queued(self, limit=10):
        return []


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
        ingestion=IngestionSettings(
            ingest_file_root="/tmp/t19_ingest",
            callback_host_allowlist="localhost,127.0.0.1",
        ),
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
    # --- path containment rules (CR-REG-36) ---
    root = Path("/tmp/t19_ingest")
    ok = resolve_within_root(root, "docs/manual.md")
    assert str(ok).startswith(str(root.resolve()))
    for bad in ("../secret.md", "/etc/passwd", "a\\b.md", "", "docs/../../x.md"):
        try:
            resolve_within_root(root, bad)
            raise SystemExit(f"must reject path: {bad!r}")
        except PathRuleError:
            pass

    # --- callback URL rules (CR-REG-36, SSRF defence) ---
    assert validate_callback_url("https://localhost:9000/cb", ("localhost", "127.0.0.1"))
    assert validate_callback_url("http://127.0.0.1:9000/cb", ("localhost", "127.0.0.1"))
    for bad_url in (
        "ftp://localhost/cb",
        "https://evil.example.com/cb",
        "http://10.0.0.5/cb",
        "https:///cb",
    ):
        try:
            validate_callback_url(bad_url, ("localhost", "127.0.0.1"))
            raise SystemExit(f"must reject url: {bad_url}")
        except PathRuleError:
            pass

    # --- idempotent acceptance (CR-ING-03) ---
    async def acceptance_flow() -> None:
        use_case = wiring.build_ingestion_acceptance(
            build_settings(), pool=object()  # FakeStore injected below instead
        ) if False else None

    from alert_agent.application.ingestion.accept import AcceptIngestion

    async def flow() -> None:
        use_case = AcceptIngestion(
            store=FakeStore(),
            ingest_root=root,
            host_allowlist=("localhost", "127.0.0.1"),
        )
        dto = IngestRequestDto.model_validate(
            {
                "docId": "doc1",
                "fileName": "manual.md",
                "title": "运维手册",
                "callbackUrl": "http://127.0.0.1:9000/cb",
                "filePath": "docs/manual.md",
            }
        )
        job1 = await use_case.execute(**to_accept_kwargs(dto))
        assert job1.status == "QUEUED" and job1.doc_id == "doc1"
        job2 = await use_case.execute(**to_accept_kwargs(dto))
        assert job2.doc_id == "doc1" and job1 is job2  # same instance returned

        # rule violation -> PathRuleError (transport maps to 400)
        bad_dto = IngestRequestDto.model_validate(
            {
                "docId": "doc2",
                "fileName": "x.md",
                "title": "t",
                "callbackUrl": "https://evil.example.com/cb",
                "filePath": "docs/x.md",
            }
        )
        try:
            await use_case.execute(**to_accept_kwargs(bad_dto))
            raise SystemExit("non-allowlisted callback must fail")
        except PathRuleError:
            pass

    asyncio.run(flow())

    # --- app wiring: /ingest routes registered, token enforced, 503 when unwired ---
    app = create_app(build_settings())
    client = TestClient(app, raise_server_exceptions=False)
    response = client.post(
        "/ingest",
        json={
            "docId": "doc1",
            "fileName": "manual.md",
            "title": "运维手册",
            "callbackUrl": "http://127.0.0.1:9000/cb",
            "filePath": "docs/manual.md",
        },
        headers={"X-Internal-Token": "real-internal-token"},
    )
    assert response.status_code == 503 and response.json()["detail"]["code"] == "NOT_READY", response.text
    response = client.post("/ingest", json={"docId": "doc1"})
    assert response.status_code == 401
    response = client.get(
        "/ingest/doc1/status", headers={"X-Internal-Token": "real-internal-token"}
    )
    assert response.status_code == 503  # route registered (404 would mean missing)

    print("T19 static verification OK")


main_sync()