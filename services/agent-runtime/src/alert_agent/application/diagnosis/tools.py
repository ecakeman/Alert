"""五种动作类型的诊断工具执行器（design 3.1，T13）。

实现 ``ToolExecutor`` 端口：时间轴读取严格限制在冻结边界之内、经由
GetTaskContext（CR-DIAG-10），故障检索经由 ``FaultDocSearcher`` 端口
（CR-DIAG-12），知识调用失败记录在状态中而不是抛出（CR-DIAG-16），
MCP 调用先用实时目录校验工具名（CR-DIAG-06）。未接线的可选端口显式
失败——绝不使用编造的结果（CR-DISC-05）。
"""

from __future__ import annotations

import hashlib
import logging
from typing import Any

from alert_agent.application.knowledge.ports import FaultDocSearcher, KnowledgeInvoker
from alert_agent.application.mcp.ports import McpSession
from alert_agent.application.task.ports import TaskGateway
from alert_agent.domain.diagnosis.actions import Action
from alert_agent.domain.diagnosis.records import KnowledgeCallRecord, McpToolResult
from alert_agent.domain.diagnosis.state import DiagnosisState
from alert_agent.domain.knowledge.models import (
    InvocationConstraints,
    InvocationContext,
    InvocationV1,
)
from alert_agent.domain.shared import ErrorCode, FatalError, RetryableError

LOGGER = logging.getLogger(__name__)


class DiagnosisToolExecutor:
    """基于网关/知识/MCP/故障检索端口的五动作执行器。"""

    def __init__(
        self,
        *,
        gateway: TaskGateway,
        knowledge: KnowledgeInvoker | None = None,
        mcp: McpSession | None = None,
        fault_searcher: FaultDocSearcher | None = None,
    ) -> None:
        self._gateway = gateway
        self._knowledge = knowledge
        self._mcp = mcp
        self._fault_searcher = fault_searcher

    async def execute(self, state: DiagnosisState, action: Action) -> dict[str, Any]:
        if action.type == "query_timeline":
            return await self._query_timeline(state)
        if action.type == "search_faults":
            return await self._search_faults(state, action)
        if action.type == "invoke_knowledge_agent":
            return await self._invoke_knowledge(state, action)
        if action.type == "call_mcp_tool":
            return await self._call_mcp_tool(state, action)
        # finish 由路由处理（写入边界）；以其他/未知类型到达此处的
        # 属于确定性的输入错误。
        raise FatalError(ErrorCode.INPUT_INVALID, f"action type is not executable here: {action.type}")

    async def _query_timeline(self, state: DiagnosisState) -> dict[str, Any]:
        """CR-DIAG-10：范围只来自任务身份携带的冻结边界；GetTaskContext
        在 Go 侧校验它（OUT_OF_RANGE）。当前 task_gateway 契约只返回
        boundary 字段——尚无记录通道，因此时间轴保持显式为空
        （"无记录"是可见的决策事实，6.3.3.2）；
        参见 T13 契约缺口披露。"""
        context = await self._gateway.get_task_context(state.identity)
        boundary = state.boundary
        diverges = (
            context.input_version_from != boundary.input_version_from
            or context.input_version_to != boundary.input_version_to
            or context.task_type != state.identity.task_type
        )
        if diverges:
            raise FatalError(
                ErrorCode.BOUNDARY_OUT_OF_RANGE,
                "task context diverges from the frozen boundary submitted with the task",
            )
        return {"timeline_records": []}

    async def _search_faults(self, state: DiagnosisState, action) -> dict[str, Any]:
        if self._fault_searcher is None:
            raise FatalError(
                ErrorCode.INPUT_INVALID,
                "fault document searcher is not wired yet (lands with the archive batch T14)",
            )
        hits = await self._fault_searcher.search(
            state.service_id, action.query, top_k=5
        )
        return {"search_hits": hits}

    async def _invoke_knowledge(self, state: DiagnosisState, action) -> dict[str, Any]:
        if self._knowledge is None:
            raise FatalError(
                ErrorCode.INPUT_INVALID,
                "knowledge invoker is not wired yet (lands with the A2A batch T22)",
            )
        invocation = InvocationV1(
            invocation_id=self._invocation_id(state, action.query),
            skill="alert_diagnosis",
            query=action.query,
            context=InvocationContext(
                episode_id=state.identity.episode_id, service_id=state.service_id
            ),
            constraints=InvocationConstraints(top_k=action.top_k),
        )
        try:
            hits = await self._knowledge.invoke(invocation)
        except (RetryableError, FatalError) as exc:
            # CR-DIAG-16：知识调用失败是状态而非任务失败——
            # 它在下一次决策提示词和 unknowns 中保持可见。
            error = f"{exc.code.value}: {exc.message}"
            LOGGER.warning("knowledge invocation failed: %s", error)
            record = KnowledgeCallRecord(results=(), error=error, called=True)
            return {"knowledge": record}
        record = KnowledgeCallRecord(results=tuple(hits), error=None, called=True)
        return {"knowledge": record}

    async def _call_mcp_tool(self, state: DiagnosisState, action) -> dict[str, Any]:
        if self._mcp is None:
            raise FatalError(
                ErrorCode.INPUT_INVALID,
                "MCP session is not wired yet (lands with the MCP batch T21)",
            )
        directory = {tool.name for tool in await self._mcp.list_tools()}
        if action.name not in directory:
            raise FatalError(
                ErrorCode.INPUT_INVALID,
                f"tool {action.name} is not in the live mcp_tools directory",
            )
        content = await self._mcp.call_tool(action.name, action.arguments)
        result = McpToolResult(
            name=action.name, arguments=action.arguments, content=tuple(content)
        )
        return {"mcp_results": [result]}

    @staticmethod
    def _invocation_id(state: DiagnosisState, query: str) -> str:
        """幂等键载体：task_id + generation + query_hash
        （CR-KNOW-04）打包进 InvocationV1 标识符。"""
        query_hash = hashlib.sha256(query.encode("utf-8")).hexdigest()[:16]
        return f"{state.identity.task_id}-{state.identity.generation}-{query_hash}"