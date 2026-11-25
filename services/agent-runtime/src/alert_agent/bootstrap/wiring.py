"""Bootstrap wiring for the persistence batch (T05).

Assembles the async pool, the Postgres receipt store, the submit use case and
the Postgres readiness probe; provides the runtime Clock/IdGen implementations.
Registering the probe into ``app.state.ready_checks`` is scheduled with the
gateway/worker batches (T07/T08) per the task plan -- this module only defines
the wiring and never touches lifespan directly.
"""

from __future__ import annotations

from datetime import UTC, datetime
from pathlib import Path
from uuid import uuid4

import grpc.aio
from psycopg_pool import AsyncConnectionPool

from alert_agent.application.archive.execute import ExecuteArchive
from alert_agent.application.diagnosis.execute import ExecuteDiagnosis
from alert_agent.application.diagnosis.tools import DiagnosisToolExecutor
from alert_agent.application.ingestion.accept import AcceptIngestion
from alert_agent.application.ingestion.execute import ProcessIngestion
from alert_agent.application.knowledge.ports import FaultDocSearcher, KnowledgeInvoker
from alert_agent.application.mcp.ports import McpSession
from alert_agent.application.retrieval.execute import ChatAnswer
from alert_agent.application.retrieval.nodes import RetrievalNodes
from alert_agent.application.retrieval.searcher import DualLegSearcher
from alert_agent.application.shared.ports import ArtifactWriter
from alert_agent.application.task.executor import TaskExecutor
from alert_agent.application.task.ports import (
    ArtifactStore,
    ReceiptStore,
    TaskGateway,
    TaskSubmission,
)
from alert_agent.application.task.submit import SubmitTask
from alert_agent.bootstrap.ingest_worker import IngestWorkerRuntime
from alert_agent.config import Settings
from alert_agent.domain.retrieval.budget import RetrievalBudget
from alert_agent.domain.shared import ErrorCode, FatalError
from alert_agent.infrastructure.a2a import build_agent_card
from alert_agent.infrastructure.archive import ArtifactFileReader, LlmFaultIndexExtractor
from alert_agent.infrastructure.artifactstore.writer import FileSystemArtifactWriter
from alert_agent.infrastructure.bm25 import InProcessBm25Index
from alert_agent.infrastructure.embedding import OpenAiCompatibleEmbedder
from alert_agent.infrastructure.grpc.gateway_client import SystemTaskGatewayClient
from alert_agent.infrastructure.ingestion import (
    DeterministicChunker,
    HttpCallbackSender,
    IngestFileLoader,
    PostgresIngestionWriter,
)
from alert_agent.infrastructure.knowledge import A2aKnowledgeClient
from alert_agent.infrastructure.llm.client import OpenAiCompatibleLlmClient
from alert_agent.infrastructure.mcp import McpSessionManager
from alert_agent.infrastructure.persistence.postgres.artifact_repo import PostgresArtifactStore
from alert_agent.infrastructure.persistence.postgres.fault_doc_repo import PostgresFaultDocStore
from alert_agent.infrastructure.persistence.postgres.ingest_repo import PostgresIngestJobStore
from alert_agent.infrastructure.persistence.postgres.pool import build_pool
from alert_agent.infrastructure.persistence.postgres.receipt_repo import PostgresReceiptStore
from alert_agent.infrastructure.persistence.postgres.retrieval_repo import (
    PostgresParentFetcher,
    PostgresVectorStore,
)
from alert_agent.infrastructure.rerank import OpenAiCompatibleReranker

__all__ = [
    "SystemClock",
    "TaskTypeDispatcher",
    "UuidIdGen",
    "build_archive_executor",
    "build_artifact_store",
    "build_artifact_writer",
    "build_chat_answer",
    "build_diagnosis_executor",
    "build_gateway_channel",
    "build_gateway_client",
    "build_ingestion_acceptance",
    "build_ingestion_worker",
    "build_llm_client",
    "build_pool",
    "build_receipt_store",
    "build_retrieval_searcher",
    "build_submit_use_case",
    "build_task_executor",
    "postgres_ready_check",
    "register_task_executor",
    "resolve_task_executor",
]


class SystemClock:
    """Timezone-aware UTC clock (application ``Clock`` port)."""

    def now(self) -> datetime:
        return datetime.now(UTC)


class UuidIdGen:
    """UUID4-based identifier generator (application ``IdGen`` port)."""

    def new_id(self, prefix: str) -> str:
        return f"{prefix}_{uuid4().hex}"


def build_receipt_store(pool: AsyncConnectionPool) -> ReceiptStore:
    return PostgresReceiptStore(pool)


def build_submit_use_case(pool: AsyncConnectionPool) -> SubmitTask:
    return SubmitTask(build_receipt_store(pool), SystemClock())


async def postgres_ready_check(pool: AsyncConnectionPool) -> None:
    """Real Postgres probe for /ready (CR-BOOT-08); raises on any failure."""
    async with pool.connection() as connection:
        await connection.execute("SELECT 1")


def build_gateway_channel(settings: Settings) -> grpc.aio.Channel:
    """Lazy-connecting aio channel to the Go control-plane gateway (AGENT_GRPC_TARGET)."""
    return grpc.aio.insecure_channel(settings.gateway.agent_grpc_target)


def build_gateway_client(channel: grpc.aio.Channel, settings: Settings) -> TaskGateway:
    """SystemTaskGateway client; the timeout derives from the single
    GATEWAY_RPC_TIMEOUT_MS setting (CR-REG-32) and retries from
    GATEWAY_RPC_MAX_RETRIES. Wired into the lifespan with the servicer batch
    (T07); worker/heartbeat consumers land with T08."""
    return SystemTaskGatewayClient(
        channel,
        timeout_seconds=settings.gateway.gateway_rpc_timeout_ms / 1000.0,
        max_retries=settings.gateway.gateway_rpc_max_retries,
    )

def build_artifact_writer(settings: Settings) -> ArtifactWriter:
    """Filesystem artifact writer rooted at ARTIFACT_STORAGE_ROOT (D5, design 4.6)."""
    return FileSystemArtifactWriter(Path(settings.storage.artifact_storage_root))


def build_artifact_store(pool: AsyncConnectionPool) -> ArtifactStore:
    """Episode-artifact version registry on cognitive.episode_artifacts (CR-REG-46)."""
    return PostgresArtifactStore(pool, UuidIdGen())

def build_llm_client(settings: Settings) -> OpenAiCompatibleLlmClient:
    """OpenAI-compatible client implementing the LlmDecider port (T11);
    complete_json/stream serve the RAG side once its ports land (T15~T17)."""
    return OpenAiCompatibleLlmClient(settings.llm)

def build_mcp_session(settings: Settings) -> McpSessionManager:
    """Assemble the MCP singleton session (T21); the per-call timeout derives
    from the single gateway RPC setting (CR-REG-32), and the endpoint/token
    come from the diagnosis group."""
    return McpSessionManager(
        endpoint=settings.diagnosis.mcp_endpoint,
        token=settings.diagnosis.mcp_token,
        timeout_seconds=settings.gateway.gateway_rpc_timeout_ms / 1000.0,
    )


def build_knowledge_client(settings: Settings) -> A2aKnowledgeClient:
    """Assemble the A2A knowledge client (T22, CR-KNOW-05): the timeout
    derives from the single gateway RPC setting; the internal token
    authenticates the knowledge agent endpoint."""
    return A2aKnowledgeClient(
        endpoint=settings.diagnosis.knowledge_a2a_endpoint,
        token=settings.gateway.internal_token,
        timeout_seconds=settings.gateway.gateway_rpc_timeout_ms / 1000.0,
    )


def build_agent_card_endpoint() -> dict:
    """AgentCard payload for the discovery endpoint (T22)."""
    return build_agent_card().model_dump()


def build_diagnosis_executor(
    settings: Settings,
    pool: AsyncConnectionPool,
    channel: grpc.aio.Channel,
    *,
    knowledge: KnowledgeInvoker | None = None,
    mcp: McpSession | None = None,
    fault_searcher: FaultDocSearcher | None = None,
) -> ExecuteDiagnosis:
    """Assemble the DIAGNOSIS TaskExecutor (T13). knowledge stays unwired
    until T22; MCP is wired by default since T21 (CR-GW-05); fault-search
    fails explicitly until T14 lands its store (CR-DISC-05)."""
    gateway = build_gateway_client(channel, settings)
    return ExecuteDiagnosis(
        gateway=gateway,
        artifact_store=build_artifact_store(pool),
        artifact_writer=build_artifact_writer(settings),
        decider=build_llm_client(settings),
        tool_executor=DiagnosisToolExecutor(
            gateway=gateway,
            knowledge=knowledge if knowledge is not None else build_knowledge_client(settings),
            mcp=mcp if mcp is not None else build_mcp_session(settings),
            fault_searcher=PostgresFaultDocStore(pool, UuidIdGen()),
        ),
        settings=settings,
        clock=SystemClock(),
    )


_EXECUTOR: TaskExecutor | None = None


def register_task_executor(executor: TaskExecutor) -> None:
    """Register the task executor before the worker runtime is built (T13);
    the worker's late binding (T08) reads it exactly once at startup."""
    global _EXECUTOR
    _EXECUTOR = executor


def resolve_task_executor() -> TaskExecutor | None:
    """Late-bound lookup consumed by worker.build_task_worker_runtime (T08)."""
    return _EXECUTOR

class TaskTypeDispatcher(TaskExecutor):
    """Dispatches by task_type to the wired diagnosis/archive executors;
    an unwired type fails deterministically instead of executing (CR-DISC-05)."""

    def __init__(self, executors: dict[str, TaskExecutor]) -> None:
        self._executors = executors

    async def execute(self, submission: TaskSubmission) -> None:
        executor = self._executors.get(submission.identity.task_type)
        if executor is None:
            raise FatalError(
                ErrorCode.INPUT_INVALID,
                f"no executor wired for task type {submission.identity.task_type}",
            )
        await executor.execute(submission)


def build_archive_executor(
    settings: Settings, pool: AsyncConnectionPool, channel: grpc.aio.Channel
) -> ExecuteArchive:
    """Assemble the ARCHIVE TaskExecutor (T14): LLM-structured extraction
    (CR-ARCH-04 llm_structured strategy), artifact body reading and the
    fault-document store."""
    gateway = build_gateway_client(channel, settings)
    return ExecuteArchive(
        gateway=gateway,
        artifact_store=build_artifact_store(pool),
        reader=ArtifactFileReader(Path(settings.storage.artifact_storage_root)),
        extractor=LlmFaultIndexExtractor(build_llm_client(settings)),
        fault_doc_store=PostgresFaultDocStore(pool, UuidIdGen()),
        clock=SystemClock(),
    )


def build_task_executor(
    settings: Settings, pool: AsyncConnectionPool, channel: grpc.aio.Channel
) -> TaskExecutor:
    """DIAGNOSIS + ARCHIVE dispatcher registered into the worker (T13/T14)."""
    return TaskTypeDispatcher(
        {
            "DIAGNOSIS": build_diagnosis_executor(settings, pool, channel),
            "ARCHIVE": build_archive_executor(settings, pool, channel),
        }
    )

def build_retrieval_searcher(settings: Settings, pool: AsyncConnectionPool) -> DualLegSearcher:
    """Assemble the dual-leg searcher (T16). The BM25 index is the single
    in-process instance per process (D9); disabled embedding leaves the
    vector leg unconfigured and reported in leg_status (CR-RAG-06)."""
    embedder = (
        OpenAiCompatibleEmbedder(settings.llm) if settings.llm.enable_embedding else None
    )
    reranker = (
        OpenAiCompatibleReranker(settings.llm) if settings.llm.enable_rerank else None
    )
    return DualLegSearcher(
        embedder=embedder,
        vector_store=PostgresVectorStore(pool) if embedder else None,
        bm25_index=InProcessBm25Index(pool),
        reranker=reranker,
        parent_fetcher=PostgresParentFetcher(pool),
        vector_top_k=8,
        fusion_top_k=10,
        rerank_top_k=5,
    )

def build_chat_answer(settings: Settings, pool: AsyncConnectionPool) -> ChatAnswer:
    """Assemble the RAG chat use case (T17); the graph itself compiles once
    behind the module-level singleton in application/retrieval/graph.py."""
    budget = RetrievalBudget(
        max_hops=settings.retrieval.retrieval_max_hops,
        max_llm_calls=settings.retrieval.retrieval_max_llm_calls,
        deadline_seconds=settings.retrieval.retrieval_deadline_seconds,
        max_zero_new_hits=settings.retrieval.retrieval_max_zero_new_hits,
    )
    llm_client = build_llm_client(settings)
    nodes = RetrievalNodes(
        searcher=build_retrieval_searcher(settings, pool),
        llm_json=llm_client,
        streamer=llm_client,
        budget=budget,
        refusal_message=settings.retrieval.refusal_message,
        grade_threshold=settings.retrieval.grade_threshold,
        clock=SystemClock(),
    )
    return ChatAnswer(nodes=nodes, clock=SystemClock())


def build_ingestion_acceptance(settings: Settings, pool: AsyncConnectionPool):
    """Assemble POST /ingest acceptance (T19): persisted job store + path/
    callback rules rooted at INGEST_FILE_ROOT (CR-ING-01/02, CR-REG-36)."""
    allowlist = tuple(
        entry.strip()
        for entry in settings.ingestion.callback_host_allowlist.split(",")
        if entry.strip()
    )
    return AcceptIngestion(
        store=PostgresIngestJobStore(pool),
        ingest_root=Path(settings.ingestion.ingest_file_root),
        host_allowlist=allowlist,
    )

def build_ingestion_worker(settings: Settings, pool: AsyncConnectionPool):
    """Assemble the ingestion execution stack (T20): loader/chunker/embedder/
    single-transaction writer + three-state callback sender (CR-ING-03~05)."""
    allowlist = tuple(
        entry.strip()
        for entry in settings.ingestion.callback_host_allowlist.split(",")
        if entry.strip()
    )
    embedder = (
        OpenAiCompatibleEmbedder(settings.llm) if settings.llm.enable_embedding else None
    )
    process = ProcessIngestion(
        store=PostgresIngestJobStore(pool),
        loader=IngestFileLoader(Path(settings.ingestion.ingest_file_root)),
        chunker=DeterministicChunker(
            chunk_size_tokens=settings.ingestion.chunk_size_tokens,
            overlap_tokens=settings.ingestion.chunk_overlap_tokens,
        ),
        embedder=embedder,
        writer=PostgresIngestionWriter(pool),
        callbacks=HttpCallbackSender(internal_token=settings.gateway.internal_token),
        host_allowlist=allowlist,
    )
    return IngestWorkerRuntime(
        store=PostgresIngestJobStore(pool),
        process=process,
        poll_interval_seconds=settings.ingestion.ingest_poll_interval_seconds,
    )