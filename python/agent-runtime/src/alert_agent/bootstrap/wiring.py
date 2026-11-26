"""持久化批次的 bootstrap 装配（T05）。

装配异步连接池、Postgres 回执存储、提交用例和 Postgres 就绪探针；提供
运行时的 Clock/IdGen 实现。按任务计划，将探针注册进
``app.state.ready_checks`` 安排在网关/worker 批次（T07/T08）——本模块只做
装配定义，绝不直接触碰 lifespan。
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
from alert_agent.infrastructure.observe import BoundedObservationSink
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
    "build_observation_sink",
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
    """带时区的 UTC 时钟（application ``Clock`` 端口）。"""

    def now(self) -> datetime:
        return datetime.now(UTC)


class UuidIdGen:
    """基于 UUID4 的标识符生成器（application ``IdGen`` 端口）。"""

    def new_id(self, prefix: str) -> str:
        return f"{prefix}_{uuid4().hex}"


def build_receipt_store(pool: AsyncConnectionPool) -> ReceiptStore:
    return PostgresReceiptStore(pool)


def build_submit_use_case(pool: AsyncConnectionPool) -> SubmitTask:
    return SubmitTask(build_receipt_store(pool), SystemClock())


async def postgres_ready_check(pool: AsyncConnectionPool) -> None:
    """供 /ready 使用的真实 Postgres 探针（CR-BOOT-08）；任何失败都会抛出。"""
    async with pool.connection() as connection:
        await connection.execute("SELECT 1")


def build_gateway_channel(settings: Settings) -> grpc.aio.Channel:
    """连接 Go 控制面网关的惰性连接 aio channel（AGENT_GRPC_TARGET）。"""
    return grpc.aio.insecure_channel(settings.gateway.agent_grpc_target)


def build_gateway_client(channel: grpc.aio.Channel, settings: Settings) -> TaskGateway:
    """SystemTaskGateway 客户端；超时取自唯一的 GATEWAY_RPC_TIMEOUT_MS
    设置（CR-REG-32），重试次数取自 GATEWAY_RPC_MAX_RETRIES。随 servicer
    批次（T07）接入 lifespan；worker/心跳消费方随 T08 落地。"""
    return SystemTaskGatewayClient(
        channel,
        timeout_seconds=settings.gateway.gateway_rpc_timeout_ms / 1000.0,
        max_retries=settings.gateway.gateway_rpc_max_retries,
    )

def build_artifact_writer(settings: Settings) -> ArtifactWriter:
    """以 ARTIFACT_STORAGE_ROOT 为根的文件系统工件写入器（D5，design 4.6）。"""
    return FileSystemArtifactWriter(Path(settings.storage.artifact_storage_root))


def build_artifact_store(pool: AsyncConnectionPool) -> ArtifactStore:
    """cognitive.episode_artifacts 上的剧集工件版本注册表（CR-REG-46）。"""
    return PostgresArtifactStore(pool, UuidIdGen())

def build_llm_client(settings: Settings) -> OpenAiCompatibleLlmClient:
    """实现 LlmDecider 端口的 OpenAI 兼容客户端（T11）；
    待 RAG 侧端口落地后，complete_json/stream 随之服务 RAG（T15~T17）。"""
    return OpenAiCompatibleLlmClient(settings.llm)

def build_mcp_session(settings: Settings) -> McpSessionManager:
    """装配 MCP 单例会话（T21）；单次调用超时取自唯一的 gateway RPC
    设置（CR-REG-32），endpoint/token 来自 diagnosis 配置组。"""
    return McpSessionManager(
        endpoint=settings.diagnosis.mcp_endpoint,
        token=settings.diagnosis.mcp_token,
        timeout_seconds=settings.gateway.gateway_rpc_timeout_ms / 1000.0,
    )


def build_knowledge_client(settings: Settings) -> A2aKnowledgeClient:
    """装配 A2A knowledge 客户端（T22，CR-KNOW-05）：超时取自唯一的
    gateway RPC 设置；内部令牌用于对 knowledge agent 端点鉴权。"""
    return A2aKnowledgeClient(
        endpoint=settings.diagnosis.knowledge_a2a_endpoint,
        token=settings.gateway.internal_token,
        timeout_seconds=settings.gateway.gateway_rpc_timeout_ms / 1000.0,
    )


def build_agent_card_endpoint() -> dict:
    """发现端点使用的 AgentCard 载荷（T22）。"""
    return build_agent_card().model_dump()


def build_observation_sink(pool: AsyncConnectionPool) -> BoundedObservationSink:
    """装配有界观测池（T23，CR-REG-19）：主路径只做入队；后台任务将
    批量数据刷写进 llm_calls。"""
    return BoundedObservationSink(pool)


def build_diagnosis_executor(
    settings: Settings,
    pool: AsyncConnectionPool,
    channel: grpc.aio.Channel,
    *,
    knowledge: KnowledgeInvoker | None = None,
    mcp: McpSession | None = None,
    fault_searcher: FaultDocSearcher | None = None,
) -> ExecuteDiagnosis:
    """装配 DIAGNOSIS TaskExecutor（T13）。knowledge 在 T22 之前保持
    未接线；MCP 自 T21 起默认接线（CR-GW-05）；fault-search 在 T14 落地
    其存储之前显式失败（CR-DISC-05）。"""
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
    """在构建 worker 运行时之前注册任务执行器（T13）；
    worker 的延迟绑定（T08）在启动时恰好读取一次。"""
    global _EXECUTOR
    _EXECUTOR = executor


def resolve_task_executor() -> TaskExecutor | None:
    """供 worker.build_task_worker_runtime（T08）消费的延迟绑定查询。"""
    return _EXECUTOR

class TaskTypeDispatcher(TaskExecutor):
    """按 task_type 分发到已接线的 diagnosis/archive 执行器；
    未接线的类型将确定性地失败而非执行（CR-DISC-05）。"""

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
    """装配 ARCHIVE TaskExecutor（T14）：LLM 结构化抽取
    （CR-ARCH-04 llm_structured 策略）、工件正文读取以及故障文档存储。"""
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
    """注册进 worker 的 DIAGNOSIS + ARCHIVE 分发器（T13/T14）。"""
    return TaskTypeDispatcher(
        {
            "DIAGNOSIS": build_diagnosis_executor(settings, pool, channel),
            "ARCHIVE": build_archive_executor(settings, pool, channel),
        }
    )

def build_retrieval_searcher(settings: Settings, pool: AsyncConnectionPool) -> DualLegSearcher:
    """装配双路检索器（T16）。BM25 索引为每进程唯一的进程内实例（D9）；
    embedding 被禁用时向量路保持未配置，并在 leg_status 中报告
    （CR-RAG-06）。"""
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
    """装配 RAG chat 用例（T17）；图本身在
    application/retrieval/graph.py 的模块级单例后只编译一次。"""
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
    """装配 POST /ingest 受理（T19）：持久化 job 存储 + 以
    INGEST_FILE_ROOT 为根的路径/回调规则（CR-ING-01/02，CR-REG-36）。"""
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
    """装配 ingestion 执行栈（T20）：loader/chunker/embedder/
    单事务写入器 + 三态回调发送器（CR-ING-03~05）。"""
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