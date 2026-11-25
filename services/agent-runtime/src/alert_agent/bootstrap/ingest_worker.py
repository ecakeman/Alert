"""Ingestion worker 运行时（T20，CR-ING-03）：单个循环轮询 QUEUED 状态的
job 并逐个执行；运行中崩溃会使 job 在持久化表中停留在 PROCESSING 状态，
恢复由重新分发/受理语义决定——不存在内存中的权威状态（CR-ING-02）。异常
绝不会杀死循环（与 task worker 相同的隔离纪律，CR-TASK-09/CR-REG-47）。"""

from __future__ import annotations

import asyncio
import logging

from alert_agent.application.ingestion.execute import ProcessIngestion
from alert_agent.application.ingestion.ports import IngestJobStore

LOGGER = logging.getLogger(__name__)


class IngestWorkerRuntime:
    """掌管 QUEUED 状态 ingest job 的轮询-执行循环（T20）。"""

    def __init__(
        self, *, store: IngestJobStore, process: ProcessIngestion, poll_interval_seconds: float
    ) -> None:
        self._store = store
        self._process = process
        self._poll_interval = poll_interval_seconds

    async def run_forever(self) -> None:
        while True:
            try:
                jobs = await self._store.list_queued(limit=10)
                for job in jobs:
                    try:
                        await self._process.execute(job)
                    except asyncio.CancelledError:
                        raise
                    except Exception:
                        # 单个 job 的失败被隔离；循环继续（CR-TASK-09）。
                        LOGGER.exception("ingest job %s crashed the runner", job.doc_id)
            except asyncio.CancelledError:
                raise
            except Exception:
                LOGGER.exception("ingest poll cycle failed")  # CR-REG-47
            await asyncio.sleep(self._poll_interval)