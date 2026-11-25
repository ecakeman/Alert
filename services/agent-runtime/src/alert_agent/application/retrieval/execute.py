"""RAG 问答/检索用例入口（T17，design 3.2.2，CR-RAG-15）。

``ChatAnswer`` 驱动一次 POST /chat 调用（HTTP SSE 接线随 T23 落地）：
以校验后的 ChatRequest 作为初始状态，使用编译好的单例图，
``emit`` 与 ``started_at`` 通过 runnable config 逐次传入，
递归上限由预算推导（CR-RAG-13）。emit 契约是一个接收
``(event_name, payload_dict)`` 的异步可调用对象。
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
    """端到端处理一轮问答；返回终态的检索状态。"""

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