"""Run-role wiring split: grpc / http / worker (CR-BOOT-04, ST-Boot-01/02).

Each builder returns a self-contained role object; the entrypoint selects the
role combination (D4 single-process dual server in this phase). Builders are
called by ``lifespan``/``app.main`` so nothing here is dead code; bodies grow
in their owning tasks (servicer registration T07, worker loops T08).
"""

from __future__ import annotations

import asyncio
import logging
from dataclasses import dataclass, field

import grpc.aio
from fastapi import FastAPI

from alert_agent.config import Settings

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
        """Start background loops; loop registration lands with the worker batch (T08)."""

    async def stop(self) -> None:
        for task in self.tasks:
            task.cancel()
        if self.tasks:
            await asyncio.gather(*self.tasks, return_exceptions=True)
        self.tasks.clear()


def build_grpc_role(settings: Settings) -> GrpcRole:
    """Create the grpc.aio server and bind it; servicers register in T07."""
    server = grpc.aio.server()
    bound = server.add_insecure_port(settings.gateway.agent_grpc_listen)
    if bound == 0:
        raise RuntimeError(f"failed to bind gRPC listen address {settings.gateway.agent_grpc_listen}")
    return GrpcRole(server=server, listen=settings.gateway.agent_grpc_listen)


def build_worker_role(settings: Settings) -> WorkerRole:
    """Assemble the worker role; queue/heartbeat/recovery loops land in T08."""
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