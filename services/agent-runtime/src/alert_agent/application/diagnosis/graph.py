"""诊断循环图编译（design 3.1.4）。

基于 pydantic ``DiagnosisState`` 载体的 StateGraph，采用节点增量合并
（列表字段通过模型的 ``ReduceAppend`` reducer 合并）。图每个进程编译一次、
按任务逐次调用；条件边通过 ``DiagnosisRouter`` 联合消费 phase + action。
终态 DONE/FAILED 的赋值保留在运行器中（执行批次，T13），图内的显式
部分结果除外。
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
    """编译受限的诊断循环（LOAD/DECIDE/TOOL/WRITE）。"""
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