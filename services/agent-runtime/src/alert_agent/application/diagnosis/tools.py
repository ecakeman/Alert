"""Diagnosis tool executors for the five action types (design 3.1, T13).

Implements the ``ToolExecutor`` port: timeline reads stay strictly inside the
frozen boundary via GetTaskContext (CR-DIAG-10), fault retrieval goes through
the ``FaultDocSearcher`` port (CR-DIAG-12), knowledge failures are recorded in
the state instead of being raised (CR-DIAG-16), and MCP calls validate the
tool name against the live directory first (CR-DIAG-06). Unwired optional
ports fail explicitly -- never with fabricated results (CR-DISC-05).
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
    """Five-action executor over gateway/knowledge/MCP/fault-search ports."""

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
        # finish is handled by routing (write boundary); reaching this point
        # with any other/unknown type is a deterministic input error.
        raise FatalError(ErrorCode.INPUT_INVALID, f"action type is not executable here: {action.type}")

    async def _query_timeline(self, state: DiagnosisState) -> dict[str, Any]:
        """CR-DIAG-10: the range comes only from the frozen boundary carried
        by the task identity; GetTaskContext validates it on the Go side
        (OUT_OF_RANGE). The current task_gateway contract returns boundary
        fields only -- no record channel exists yet, so the timeline stays
        explicitly empty ("no records" is a visible decision fact, 6.3.3.2);
        see the T13 contract-gap disclosure."""
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
            # CR-DIAG-16: a knowledge failure is state, not a task failure --
            # it stays visible in the next decision prompt and in unknowns.
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
        """Idempotency key carrier: task_id + generation + query_hash
        (CR-KNOW-04) packed into the InvocationV1 identifier."""
        query_hash = hashlib.sha256(query.encode("utf-8")).hexdigest()[:16]
        return f"{state.identity.task_id}-{state.identity.generation}-{query_hash}"