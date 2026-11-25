"""POST /chat true-streaming SSE handler (T23, design 3.2.3, CR-RAG-15~19,
CR-REG-18/38/52).

Tokens stream out the moment the model produces them (T17 synthesize node
emits through the per-request channel -- no batching, no
"collect-then-replay"); trace frames flow when each node finishes; citations
and done frames land after the graph terminates. Citations whose number never
appears in the answer are dropped (CR-RAG-19). Domain failures become an
error frame with a stable business code (CR-REG-52); the whole path is async
(CR-REG-38).
"""

from __future__ import annotations

import asyncio
import logging
import re
import time

from fastapi import HTTPException, Request
from fastapi.responses import StreamingResponse

from alert_agent.transport.http.dto.chat import ChatRequestDto
from alert_agent.transport.http.mapper.chat import to_domain_chat_request
from alert_agent.transport.sse import (
    citations_frame,
    done_frame,
    encode_event,
    error_frame,
)

LOGGER = logging.getLogger(__name__)

_CITED_NUMBER = re.compile(r"\[(\d+)\]")


def _cited_numbers(answer: str) -> set[int]:
    return {int(match) for match in _CITED_NUMBER.findall(answer)}


async def chat(request: Request, dto: ChatRequestDto) -> StreamingResponse:
    chat_case = getattr(request.app.state, "chat_answer", None)
    if chat_case is None:
        raise HTTPException(
            status_code=503,
            detail={"code": "NOT_READY", "message": "chat stack is not wired yet"},
        )
    domain_request = to_domain_chat_request(dto)
    started = time.monotonic()
    frames: asyncio.Queue[str] = asyncio.Queue()

    async def emit(event: str, data: dict) -> None:
        # Per-request channel (T17): events never cross sessions (6.5.3.3).
        frames.put_nowait(encode_event(event, data))

    async def event_stream():
        try:
            task = asyncio.create_task(chat_case.chat(domain_request, emit))
            # Forward frames as they appear; poll keeps the generator simple
            # and fully async (CR-REG-38).
            while not (task.done() and frames.empty()):
                try:
                    frame = await asyncio.wait_for(frames.get(), timeout=0.05)
                    yield frame
                except TimeoutError:
                    continue
            final = await task
            cited = _cited_numbers(final.answer)
            kept = [c for c in final.citations if c.no in cited]
            yield citations_frame(kept)
            latency_ms = (time.monotonic() - started) * 1000.0
            yield done_frame(latency_ms)
            sink = getattr(request.app.state, "observation_sink", None)
            if sink is not None:
                sink.record(scene="chat", model="stream", latency_ms=latency_ms)
        except asyncio.CancelledError:
            raise
        except Exception as exc:
            LOGGER.exception("chat stream failed")
            yield error_frame("CHAT_FAILED", f"{type(exc).__name__}: {exc}")

    return StreamingResponse(
        event_stream(),
        media_type="text/event-stream",
        headers={"Cache-Control": "no-cache", "X-Accel-Buffering": "no"},
    )