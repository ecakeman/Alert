"""RAG chat/retrieve use-case entry (T17, design 3.2.2, CR-RAG-15).

``ChatAnswer`` drives one POST /chat invocation (HTTP SSE wiring lands with
T23): initial state from the validated ChatRequest, the compiled singleton
graph, per-invocation ``emit`` and ``started_at`` through the runnable config,
and the recursion cap derived from the budget (CR-RAG-13). The emit contract
is an async callable receiving ``(event_name, payload_dict)``.
"""

from __future__ import annotations

import logging
from collections.abc import Awaitable, Callable

from alert_agent.application.retrieval.graph import NODE_COUNT, get_compiled_graph
from alert_agent.application.retrieval.nodes import RetrievalNodes
from alert_agent.application.shared.ports import Clock
from alert_agent.domain.retrieval.chat import ChatRequest
from alert_agent.domain.retrieval.state import RetrievalState

LOGGER = logging.getLogger(__name__)

EmitFn = Callable[[str, dict], Awaitable[None]]


class ChatAnswer:
    """One chat turn end to end; returns the terminal retrieval state."""

    def __init__(self, *, nodes: RetrievalNodes, clock: Clock) -> None:
        self._nodes = nodes
        self._clock = clock

    async def chat(self, request: ChatRequest, emit: EmitFn | None = None) -> RetrievalState:
        graph = await get_compiled_graph(lambda: self._nodes)
        initial = RetrievalState(
            session_id=request.session_id,
            question=request.message,
            history=tuple(request.history),
            budget=self._nodes.budget,
        )
        final = await graph.ainvoke(
            initial.model_dump(),
            config={
                "recursion_limit": self._nodes.budget.recursion_limit(NODE_COUNT),
                "configurable": {
                    "emit": emit,
                    "started_at": self._clock.now(),
                },
            },
        )
        terminal = RetrievalState.model_validate(final)
        LOGGER.info(
            "chat %s finished qtype=%s verified=%s citations=%d",
            request.session_id,
            terminal.qtype,
            terminal.verified,
            len(terminal.citations),
        )
        return terminal