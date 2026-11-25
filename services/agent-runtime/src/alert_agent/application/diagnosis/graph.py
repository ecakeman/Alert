"""Diagnosis loop graph compilation (design 3.1.4).

StateGraph over the pydantic ``DiagnosisState`` carrier with node-increment
merging (list fields merge via the model's ``ReduceAppend`` reducers). The
graph is compiled once per process and invoked per task; conditional edges
consume phase + action jointly via ``DiagnosisRouter``. Terminal DONE/FAILED
assignment stays with the runner (execute batch, T13) except the in-graph
explicit partial outcome.
"""

from __future__ import annotations

from langgraph.graph import END, START, StateGraph
from langgraph.graph.state import CompiledStateGraph

from alert_agent.application.diagnosis.nodes import DiagnosisNodes
from alert_agent.application.diagnosis.routing import DiagnosisRouter
from alert_agent.domain.diagnosis.state import DiagnosisState

_DECIDE_TARGETS = {
    "run_tool": "run_tool",
    "write": "write",
    "fallback": "fallback",
    "finalize_partial": "finalize_partial",
}
_LOOP_TARGETS = {
    "decide": "decide",
    "finalize_partial": "finalize_partial",
}


def build_diagnosis_graph(
    nodes: DiagnosisNodes, router: DiagnosisRouter
) -> CompiledStateGraph:
    """Compile the constrained diagnosis loop (LOAD/DECIDE/TOOL/WRITE)."""
    graph = StateGraph(DiagnosisState)
    graph.add_node("load", nodes.load)
    graph.add_node("decide", nodes.decide)
    graph.add_node("run_tool", nodes.run_tool)
    graph.add_node("fallback", nodes.fallback)
    graph.add_node("write", nodes.write)
    graph.add_node("finalize_partial", nodes.finalize_partial)
    graph.add_edge(START, "load")
    graph.add_edge("load", "decide")
    graph.add_conditional_edges("decide", router.after_decide, _DECIDE_TARGETS)
    graph.add_conditional_edges("run_tool", router.after_tool, _LOOP_TARGETS)
    graph.add_conditional_edges("fallback", router.after_fallback, _LOOP_TARGETS)
    graph.add_edge("write", END)
    graph.add_edge("finalize_partial", END)
    return graph.compile()