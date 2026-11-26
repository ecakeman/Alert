"""单进程双服务器 lifespan（D4，design 4.1，CR-BOOT-05~07）。

启动顺序：settings 快速失败（由入口完成）-> Postgres 连接池 ->
worker 角色 -> 带 SubmitTask servicer 的 grpc.aio 服务器（T07）-> 注册就绪
检查。关闭按相反顺序在配置的预算内（<= 20 秒）执行：/ready 先行关闭，
gRPC 优雅停止，worker 循环清空，随后关闭连接池与 channel。信号处理保持在
uvicorn 原生 lifespan 机制和标准库之内——不使用私有 API 的 hack
（CR-BOOT-07）。worker 循环注册随 worker 批次落地（T08）。
"""

from __future__ import annotations

import asyncio
import logging
from collections.abc import AsyncIterator, Awaitable, Callable
from contextlib import AbstractAsyncContextManager, asynccontextmanager

from fastapi import FastAPI

from alert_agent.application.task.queue import BoundedTaskQueue
from alert_agent.bootstrap.roles import GrpcRole, WorkerRole, build_grpc_role, build_worker_role
from alert_agent.bootstrap.wiring import (
    build_chat_answer,
    build_gateway_channel,
    build_ingestion_acceptance,
    build_ingestion_worker,
    build_pool,
    build_retrieval_searcher,
    build_submit_use_case,
    build_task_executor,
    postgres_ready_check,
    register_task_executor,
)
from alert_agent.bootstrap.worker import EnqueueingSubmit, build_task_worker_runtime
from alert_agent.config import Settings

LOGGER = logging.getLogger(__name__)

ReadyCheck = Callable[[], Awaitable[None]]


def build_lifespan(settings: Settings) -> Callable[[FastAPI], AbstractAsyncContextManager[None]]:
    @asynccontextmanager
    async def lifespan(app: FastAPI) -> AsyncIterator[None]:
        pool = build_pool(settings)
        await pool.open()
        # T18：/search 处理器从 app.state 解析其检索器；该依赖在持有
        # 连接池的此处装配（已披露的白名单外行，CR-DISC-03）。
        app.state.retrieval_searcher = build_retrieval_searcher(settings, pool)
        # T19：ingest 处理器同样从 app.state 解析其受理用例
        # （已披露的白名单外行，CR-DISC-03）。
        app.state.ingestion_accept = build_ingestion_acceptance(settings, pool)
        # T23：/chat 处理器从 app.state 解析其 chat 用例
        # （已披露的白名单外行，CR-DISC-03）。
        app.state.chat_answer = build_chat_answer(settings, pool)

        # 提交路径与消费方共用一个有界队列（CR-TASK-08）；队列已满时
        # 保持回执作为"待恢复"事实并报告 accepted=false（6.1.3.1）。
        queue = BoundedTaskQueue(settings.gateway.queue_capacity)
        submit = EnqueueingSubmit(build_submit_use_case(pool), queue)
        worker_role = build_worker_role(settings)
        grpc_channel = build_gateway_channel(settings)
        # T13/T14 装配：任务类型分发器（DIAGNOSIS + ARCHIVE）在 worker
        # 运行时启动之前注册，使各循环不再空转；knowledge/MCP 在 T22/T21
        # 之前保持未接线，届时其动作将显式失败。
        register_task_executor(build_task_executor(settings, pool, grpc_channel))
        worker_runtime = build_task_worker_runtime(settings, pool, grpc_channel, queue)
        worker_role.tasks.extend(await worker_runtime.start())
        # T20：ingestion worker 循环持续轮询 QUEUED 状态的 job 直至
        # READY/FAILED（已披露的白名单外注册，CR-DISC-03）。
        ingest_worker = build_ingestion_worker(settings, pool)
        worker_role.tasks.append(
            asyncio.create_task(ingest_worker.run_forever(), name="ingest-worker")
        )
        grpc_role = build_grpc_role(settings, submit)
        await grpc_role.start()
        app.state.ready_checks.append(grpc_started_check(grpc_role))
        app.state.ready_checks.append(lambda: postgres_ready_check(pool))
        try:
            yield
        finally:
            await shutdown(app, worker_role, grpc_role, settings)
            await pool.close()
            await grpc_channel.close()

    return lifespan


def grpc_started_check(role: GrpcRole) -> ReadyCheck:
    async def check() -> None:
        if not role.started:
            raise RuntimeError("grpc server is not started")

    return check


async def shutdown(
    app: FastAPI,
    worker_role: WorkerRole,
    grpc_role: GrpcRole,
    settings: Settings,
) -> None:
    app.state.ready_checks.clear()
    budget = float(settings.shared.shutdown_timeout_seconds)
    try:
        await asyncio.wait_for(grpc_role.stop(max(budget / 2.0, 0.1)), timeout=budget)
    except TimeoutError:
        LOGGER.exception("grpc server stop exceeded the shutdown budget (%ss)", budget)
    try:
        await asyncio.wait_for(worker_role.stop(), timeout=budget)
    except TimeoutError:
        LOGGER.exception("worker shutdown exceeded the shutdown budget (%ss)", budget)