"""Bounded-queue observability sink (T23, design 5.1 observe, CR-REG-19).

The main path only enqueues into an in-memory bounded queue -- a full queue
increments a drop counter and never blocks the business flow; a background
task batches rows into cognitive.llm_calls (scene/model/latency_ms/tokens/
degraded/ts). Flush failures are logged and retried on the next cycle, never
propagated into the main path.
"""

from __future__ import annotations

import asyncio
import logging
import uuid

from psycopg_pool import AsyncConnectionPool

LOGGER = logging.getLogger(__name__)

_INSERT = """
INSERT INTO cognitive.llm_calls
    (call_id, scene, model, latency_ms, prompt_tokens, completion_tokens, degraded)
VALUES (%s, %s, %s, %s, %s, %s, %s)
"""


class BoundedObservationSink:
    """In-memory bounded queue + background batched flush (CR-REG-19)."""

    def __init__(
        self,
        pool: AsyncConnectionPool,
        *,
        capacity: int = 1000,
        batch_size: int = 50,
        flush_interval_seconds: float = 5.0,
    ) -> None:
        self._pool = pool
        self._queue: asyncio.Queue = asyncio.Queue(maxsize=capacity)
        self._batch_size = batch_size
        self._flush_interval = flush_interval_seconds
        self.dropped = 0

    def record(
        self,
        *,
        scene: str,
        model: str,
        latency_ms: float,
        prompt_tokens: int = 0,
        completion_tokens: int = 0,
        degraded: bool = False,
    ) -> None:
        entry = {
            "call_id": f"call_{uuid.uuid4().hex}",
            "scene": scene[:64],
            "model": model[:200],
            "latency_ms": max(0, int(latency_ms)),
            "prompt_tokens": prompt_tokens,
            "completion_tokens": completion_tokens,
            "degraded": degraded,
        }
        try:
            self._queue.put_nowait(entry)
        except asyncio.QueueFull:
            self.dropped += 1
            LOGGER.warning("observation queue full; entry dropped (total %d)", self.dropped)

    async def flush_forever(self) -> None:
        while True:
            batch: list[dict] = []
            try:
                batch.append(
                    await asyncio.wait_for(self._queue.get(), timeout=self._flush_interval)
                )
            except TimeoutError:
                pass
            while len(batch) < self._batch_size:
                try:
                    batch.append(self._queue.get_nowait())
                except asyncio.QueueEmpty:
                    break
            if batch:
                try:
                    await self._write(batch)
                except Exception:
                    LOGGER.exception("observation flush failed; %d rows stay for retry", len(batch))

    async def _write(self, batch: list[dict]) -> None:
        async with self._pool.connection() as connection:
            await connection.executemany(
                _INSERT,
                [
                    (
                        entry["call_id"],
                        entry["scene"],
                        entry["model"],
                        entry["latency_ms"],
                        entry["prompt_tokens"],
                        entry["completion_tokens"],
                        entry["degraded"],
                    )
                    for entry in batch
                ],
            )