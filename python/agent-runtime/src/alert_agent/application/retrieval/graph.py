"""RAG 循环图编译为模块级单例（T17，design 3.2.2，
CR-RAG-15，CR-REG-11）。

图在每个进程中仅通过双重检查锁编译一次；它不持有任何 emit 绑定——
``emit`` 与 ``started_at`` 在每次调用时通过 runnable config 传递，
因此不会发生 ``id(emit)`` 式的缓存。递归上限由预算与节点数推导
（CR-RAG-13），而不是一个独立的魔法常量。
"""

from __future__ import annotations

import asyncio
from collections.abc import Callable

from langgraph.graph import END, START, StateGraph
from langgraph.graph.state import CompiledStateGraph

from alert_agent.application.retrieval.nodes import RetrievalNodes
from alert_agent.application.retrieval.routing import RetrievalRouter
from alert_agent.domain.retrieval.state import RetrievalState

NODE_COUNT = 10  # rewrite/router/decompose/retrieve/grade/sufficiency/synthesize/verify/refuse/done

_GRAPH: CompiledStateGraph | None = None
_GRAPH_LOCK = asyncio.Lock()

_AFTER_ROUTER = {
    "decompose": "decompose",
    "retrieve": "retrieve",
    "refuse": "refuse",
}
_AFTER_GRADE = {
    "synthesize": "synthesize",
    "refuse": "refuse",
    "sufficiency": "sufficiency",
}
_AFTER_SUFFICIENCY = {"retrieve": "retrieve", "refuse": "refuse"}
_AFTER_VERIFY = {"synthesize": "synthesize", "done": "done"}


def _build(nodes: RetrievalNodes, router: RetrievalRouter) -> CompiledStateGraph:
    graph = StateGraph(RetrievalState)
    graph.add_node("rewrite", nodes.rewrite)
    graph.add_node("router", nodes.router)
    graph.add_node("decompose", nodes.decompose)
    graph.add_node("retrieve", nodes.retrieve)
    graph.add_node("grade", nodes.grade)
    graph.add_node("sufficiency", nodes.sufficiency)
    graph.add_node("synthesize", nodes.synthesize)
    graph.add_node("verify", nodes.verify)
    graph.add_node("refuse", nodes.refuse)
    graph.add_node("done", nodes.done)
    graph.add_edge(START, "rewrite")
    graph.add_edge("rewrite", "router")
    graph.add_conditional_edges("router", router.after_router, _AFTER_ROUTER)
    graph.add_edge("decompose", "retrieve")
    graph.add_edge("retrieve", "grade")
    graph.add_conditional_edges("grade", router.after_grade, _AFTER_GRADE)
    graph.add_conditional_edges("sufficiency", router.after_sufficiency, _AFTER_SUFFICIENCY)
    graph.add_edge("synthesize", "verify")
    graph.add_conditional_edges("verify", router.after_verify, _AFTER_VERIFY)
    graph.add_edge("refuse", END)
    graph.add_edge("done", END)
    return graph.compile()


async def get_compiled_graph(
    build_nodes: Callable[[], RetrievalNodes],
) -> CompiledStateGraph:
    """每个进程仅编译一次；``build_nodes`` 只在首次调用时执行。"""
    global _GRAPH
    if _GRAPH is None:
        async with _GRAPH_LOCK:
            if _GRAPH is None:
                nodes = build_nodes()
                _GRAPH = _build(nodes, RetrievalRouter(nodes.clock))
    return _GRAPH