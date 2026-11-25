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

from alert_agent.application.diagnosis.execute import ExecuteDiagnosis
from alert_agent.application.diagnosis.tools import DiagnosisToolExecutor
from alert_agent.application.knowledge.ports import FaultDocSearcher, KnowledgeInvoker
from alert_agent.application.mcp.ports import McpSession
from alert_agent.application.shared.ports import ArtifactWriter
from alert_agent.application.task.executor import TaskExecutor
from alert_agent.application.task.ports import ArtifactStore, ReceiptStore, TaskGateway
from alert_agent.application.task.submit import SubmitTask
from alert_agent.config import Settings
from alert_agent.infrastructure.artifactstore.writer import FileSystemArtifactWriter
from alert_agent.infrastructure.grpc.gateway_client import SystemTaskGatewayClient
from alert_agent.infrastructure.llm.client import OpenAiCompatibleLlmClient
from alert_agent.infrastructure.persistence.postgres.artifact_repo import PostgresArtifactStore
from alert_agent.infrastructure.persistence.postgres.pool import build_pool
from alert_agent.infrastructure.persistence.postgres.receipt_repo import PostgresReceiptStore

__all__ = [
    "SystemClock",
    "UuidIdGen",
    "build_artifact_store",
    "build_artifact_writer",
    "build_diagnosis_executor",
    "build_gateway_channel",
    "build_gateway_client",
    "build_llm_client",
    "build_pool",
    "build_receipt_store",
    "build_submit_use_case",
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

def build_diagnosis_executor(
    settings: Settings,
    pool: AsyncConnectionPool,
    channel: grpc.aio.Channel,
    *,
    knowledge: KnowledgeInvoker | None = None,
    mcp: McpSession | None = None,
    fault_searcher: FaultDocSearcher | None = None,
) -> ExecuteDiagnosis:
    """Assemble the DIAGNOSIS TaskExecutor (T13). knowledge/MCP/fault-search
    implementations land with T22/T21/T14; until then the corresponding
    actions fail explicitly instead of fabricating results (CR-DISC-05)."""
    gateway = build_gateway_client(channel, settings)
    return ExecuteDiagnosis(
        gateway=gateway,
        artifact_store=build_artifact_store(pool),
        artifact_writer=build_artifact_writer(settings),
        decider=build_llm_client(settings),
        tool_executor=DiagnosisToolExecutor(
            gateway=gateway,
            knowledge=knowledge,
            mcp=mcp,
            fault_searcher=fault_searcher,
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