"""Ingestion worker runtime (T20, CR-ING-03): one loop polls QUEUED jobs and
executes them one at a time; a crash mid-run leaves the job at PROCESSING in
the persisted table, and re-dispatch/acceptance semantics decide recovery --
no in-memory authority (CR-ING-02). Exceptions never kill the loop (same
isolation discipline as the task worker, CR-TASK-09/CR-REG-47)."""

from __future__ import annotations

import asyncio
import logging

from alert_agent.application.ingestion.execute import ProcessIngestion
from alert_agent.application.ingestion.ports import IngestJobStore

LOGGER = logging.getLogger(__name__)


class IngestWorkerRuntime:
    """Owns the poll-execute loop for QUEUED ingest jobs (T20)."""

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
                        # One job's failure isolates; the loop continues (CR-TASK-09).
                        LOGGER.exception("ingest job %s crashed the runner", job.doc_id)
            except asyncio.CancelledError:
                raise
            except Exception:
                LOGGER.exception("ingest poll cycle failed")  # CR-REG-47
            await asyncio.sleep(self._poll_interval)