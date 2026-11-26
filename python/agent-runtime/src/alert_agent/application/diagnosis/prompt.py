"""决策提示词构建（design 3.1.5，CR-DIAG-07，CR-REG-05）。

固定结构，绝不把整个状态倾倒进一条消息：系统指令、任务视图、带有显式
字符预算（截断可见）的各列表证据摘要、已执行动作表、最近一次错误与
剩余预算。用户派生的内容被转义进 XML 边界标签，注入的文本无法跨越
分区（5.3.7）；疑似注入特征由调用方标记为 ``security`` trace。
"""

from __future__ import annotations

import json
from dataclasses import dataclass
from datetime import datetime
from typing import Any

from alert_agent.domain.diagnosis.state import DiagnosisState

DECISION_SCHEMA: dict[str, Any] = {
    "type": "object",
    "required": ["type"],
    "additionalProperties": False,
    "properties": {
        "type": {
            "enum": [
                "query_timeline",
                "search_faults",
                "invoke_knowledge_agent",
                "call_mcp_tool",
                "finish",
            ]
        },
        "query": {"type": "string", "minLength": 1},
        "top_k": {"type": "integer", "minimum": 1, "maximum": 20},
        "name": {"type": "string", "minLength": 1},
        "arguments": {"type": "object"},
        "report": {
            "type": "object",
            "required": ["summary"],
            "additionalProperties": False,
            "properties": {
                "summary": {"type": "string", "minLength": 1},
                "observations": {"type": "array", "items": {"type": "string"}},
                "hypotheses": {"type": "array", "items": {"type": "string"}},
                "unknowns": {"type": "array", "items": {"type": "string"}},
                "next_steps": {"type": "array", "items": {"type": "string"}},
            },
        },
    },
}

SYSTEM_PROMPT = (
    "你是告警诊断智能体。基于任务视图与证据摘要选择下一个动作。"
    "可选动作：query_timeline（拉取冻结范围内的监控点与应用事件时间轴，无参数，"
    "范围一律使用任务冻结边界，禁止自行扩展）、search_faults（检索历史故障文档，"
    "需要非空 query）、invoke_knowledge_agent（咨询知识智能体，需要非空 query）、"
    "call_mcp_tool（调用 MCP 工具，需要工具名与 JSON 对象参数）、"
    "finish（提交最终诊断报告并结束，需要完整 report）。"
    "必须只输出一个 JSON 对象（禁止解释文字、禁止代码围栏），内容使用简体中文。"
)


@dataclass(frozen=True, slots=True)
class PromptBudgets:
    """各分区的字符/条目预算，全部来自集中式配置。"""

    timeline_chars: int
    search_chars: int
    knowledge_chars: int
    mcp_chars: int
    summary_items: int
    actions_items: int
    last_error_chars: int


@dataclass(frozen=True, slots=True)
class DecisionMessages:
    system: str
    user: str


def _escape(value: str) -> str:
    return value.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")


def _section(tag: str, content: str, budget_chars: int) -> str:
    text = _escape(content)
    if len(text) > budget_chars:
        text = text[:budget_chars] + "…[truncated]"
    return f"<{tag}>\n{text}\n</{tag}>"


def _json(value: Any) -> str:
    return json.dumps(value, ensure_ascii=False, sort_keys=True, default=str)


def build_decision_messages(
    state: DiagnosisState, *, budgets: PromptBudgets, now: datetime
) -> DecisionMessages:
    """为一次 DECIDE 轮次组装固定结构的决策提示词。"""
    boundary = state.boundary
    sections: list[str] = [
        _section(
            "task",
            json.dumps(
                {
                    "task_id": state.identity.task_id,
                    "episode_id": state.identity.episode_id,
                    "generation": state.identity.generation,
                    "service_id": state.service_id,
                    "service_name": state.service_name,
                    "input_versions": [boundary.input_version_from, boundary.input_version_to],
                    "monitor_watermark": boundary.monitor_watermark,
                    "app_event_watermark": boundary.app_event_watermark,
                    "deadline_at": boundary.deadline_at.isoformat(),
                },
                ensure_ascii=False,
            ),
            2000,
        )
    ]
    recent_timeline = state.timeline_records[-budgets.summary_items :]
    sections.append(
        _section(
            "timeline_summary",
            _json([record.model_dump(mode="json") for record in recent_timeline]),
            budgets.timeline_chars,
        )
    )
    recent_hits = state.search_hits[-budgets.summary_items :]
    sections.append(
        _section(
            "fault_hits_summary",
            _json([hit.model_dump(mode="json") for hit in recent_hits]),
            budgets.search_chars,
        )
    )
    knowledge_view = state.knowledge.model_dump(mode="json")
    sections.append(
        _section("knowledge_summary", _json(knowledge_view), budgets.knowledge_chars)
    )
    recent_mcp = state.mcp_results[-budgets.summary_items :]
    sections.append(
        _section(
            "mcp_summary",
            _json([result.model_dump(mode="json") for result in recent_mcp]),
            budgets.mcp_chars,
        )
    )
    recent_actions = state.executed_actions[-budgets.actions_items :]
    sections.append(
        _section(
            "executed_actions",
            _json([action.model_dump(mode="json") for action in recent_actions]),
            budgets.actions_items * 200,
        )
    )
    if state.last_error:
        sections.append(
            _section("last_error", state.last_error, budgets.last_error_chars)
        )
    remaining_seconds = max(0.0, (state.boundary.deadline_at - now).total_seconds())
    sections.append(
        _section(
            "budget",
            json.dumps(
                {
                    "tool_rounds_left": max(0, state.budget.max_tool_rounds - state.tool_rounds),
                    "llm_calls_left": max(0, state.budget.max_llm_calls - state.llm_calls),
                    "seconds_left_to_deadline": remaining_seconds,
                },
                ensure_ascii=False,
            ),
            500,
        )
    )
    sections.append(_section("output_schema", _json(DECISION_SCHEMA), 3000))
    return DecisionMessages(system=SYSTEM_PROMPT, user="\n\n".join(sections))