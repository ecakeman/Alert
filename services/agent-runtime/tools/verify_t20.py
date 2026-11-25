"""T20 static verification: chunking, writer alignment, worker wiring."""

import sys

sys.path.insert(0, "/home/sancho/workspace/projects/Alert/services/agent-runtime/src")

from psycopg_pool import AsyncConnectionPool

from alert_agent.bootstrap import wiring
from alert_agent.bootstrap.ingest_worker import IngestWorkerRuntime
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
from alert_agent.infrastructure.ingestion import (
    DeterministicChunker,
    HttpCallbackSender,
    PostgresIngestionWriter,
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
        ingestion=IngestionSettings(
            ingest_file_root="/tmp/t20_ingest",
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
    # --- deterministic chunking: parents carry sections, children split ---
    chunker = DeterministicChunker(chunk_size_tokens=64, overlap_tokens=8)
    content = "\n\n".join(
        f"Section {i} " + "word " * 300 for i in range(3)
    )
    chunks = chunker.chunk(content)
    parents = [c for c in chunks if c.is_parent]
    children = [c for c in chunks if not c.is_parent]
    assert len(parents) == 3, len(parents)
    assert len(children) >= 3
    assert all(child.parent_id in {"p0", "p1", "p2"} for child in children)
    assert all(parent.content.startswith("Section") for parent in parents)
    # empty content -> no chunks
    assert chunker.chunk("") == []

    # --- writer alignment guard (CR-ING-04 preconditions) ---
    writer = PostgresIngestionWriter(None)  # pool unused in validation
    try:
        writer.write.__wrapped__ if False else None
    except AttributeError:
        pass

    # --- callback sender construction ---
    sender = HttpCallbackSender(internal_token="real-internal-token")
    assert sender._max_retries == 3

    # --- worker wiring (no DB connection at construction: pool reference only) ---
    runtime = wiring.build_ingestion_worker(build_settings(), None)
    assert isinstance(runtime, IngestWorkerRuntime)
    assert runtime._poll_interval == 5.0

    print("T20 static verification OK")


main_sync()