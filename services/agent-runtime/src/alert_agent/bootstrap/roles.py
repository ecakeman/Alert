"""运行角色装配拆分：grpc / http / worker（CR-BOOT-04，ST-Boot-01/02）。

每个构建器返回一个自包含的角色对象；入口选择角色组合（本阶段为 D4
单进程双服务器）。构建器由 ``lifespan``/``app.main`` 调用，因此这里不存在
死代码；函数体随各自的任务增长（servicer 注册 T07、worker 循环 T08）。
"""

from __future__ import annotations

import asyncio
import logging
from dataclasses import dataclass, field

import grpc.aio
from fastapi import FastAPI

from alert_agent.application.task.submit import SubmitTask
from alert_agent.config import Settings
from alert_agent.infrastructure.grpc.gen.alertflow.agent.v1 import agent_task_pb2_grpc
from alert_agent.transport.grpc.servicer import AgentTaskServicer

LOGGER = logging.getLogger(__name__)


@dataclass
class GrpcRole:
    server: grpc.aio.Server
    listen: str
    started: bool = False

    async def start(self) -> None:
        await self.server.start()
        self.started = True
        LOGGER.info("grpc server listening on %s", self.listen)

    async def stop(self, grace_seconds: float) -> None:
        if not self.started:
            return
        await self.server.stop(grace_seconds)
        self.started = False


@dataclass
class WorkerRole:
    tasks: list[asyncio.Task[None]] = field(default_factory=list)

    async def start(self) -> None:
        """启动后台循环；循环注册随 worker 批次落地（T08）。"""

    async def stop(self) -> None:
        for task in self.tasks:
            task.cancel()
        if self.tasks:
            await asyncio.gather(*self.tasks, return_exceptions=True)
        self.tasks.clear()


def build_grpc_role(settings: Settings, submit: SubmitTask) -> GrpcRole:
    """创建 grpc.aio 服务器、注册 SubmitTask servicer 并完成绑定
    （T07，CR-GW-02）；该 servicer 是真实接线的依赖，不是死代码。"""
    server = grpc.aio.server()
    agent_task_pb2_grpc.add_AgentTaskServiceServicer_to_server(AgentTaskServicer(submit), server)
    bound = server.add_insecure_port(settings.gateway.agent_grpc_listen)
    if bound == 0:
        raise RuntimeError(f"failed to bind gRPC listen address {settings.gateway.agent_grpc_listen}")
    return GrpcRole(server=server, listen=settings.gateway.agent_grpc_listen)


def build_worker_role(settings: Settings) -> WorkerRole:
    """装配 worker 角色；队列/心跳/恢复循环在 T08 落地。"""
    return WorkerRole()


@dataclass
class HttpRole:
    app: FastAPI
    host: str
    port: int


def build_http_role(settings: Settings, app: FastAPI) -> HttpRole:
    host, port = split_listen(settings.gateway.http_listen)
    return HttpRole(app=app, host=host, port=port)


def split_listen(listen: str) -> tuple[str, int]:
    host, _, port = listen.rpartition(":")
    return host, int(port)