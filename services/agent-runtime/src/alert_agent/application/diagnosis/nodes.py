"""Diagnosis loop node implementations (design 3.1.4/3.1.5).

Nodes return incremental dicts only (CR-DIAG-02, CR-REG-02); phase is assigned
exactly at its owning boundary (CR-DIAG-03). Existing-version reads and
artifact writes are injected callables provided by the execution batch (T13)
so this module stays free of IO (CR-LAY-02). Every failure path records a
readable ``last_error``; nothing is swallowed (CR-REG-47).
"""

from __future__ import annotations

from collections.abc import Awaitable, Callable
from datetime import datetime
from typing import Any

from pydantic import BaseModel

from alert_agent.application.diagnosis.parsing import parse_action
from alert_agent.application.diagnosis.ports import LlmDecider, ToolExecutor
from alert_agent.application.diagnosis.prompt import (
    DECISION_SCHEMA,
    PromptBudgets,
    build_decision_messages,
)
from alert_agent.application.shared.ports import Clock
from alert_agent.domain.diagnosis.actions import Action
from alert_agent.domain.diagnosis.records import ExecutedAction
from alert_agent.domain.diagnosis.report import DiagnosisReport
from alert_agent.domain.diagnosis.state import DiagnosisState
from alert_agent.domain.shared import ErrorCode, FatalError, RetryableError

ExistingVersionsLoader = Callable[[DiagnosisState], Awaitable[dict[str, Any]]]
ArtifactWriteCallback = Callable[[DiagnosisState], Awaitable[dict[str, Any]]]

_ACTION_BRIEF_CHARS = 120


def _brief(action: Action) -> str:
    if isinstance(action, BaseModel):
        payload = action.model_dump(mode="json", exclude={"type", "report"})
        return str(payload)[:_ACTION_BRIEF_CHARS]
    return ""


class DiagnosisNodes:
    """Node bundle wired into the compiled state graph."""

    def __init__(
        self,
        *,
        decider: LlmDecider,
        executor: ToolExecutor,
        load_existing_versions: ExistingVersionsLoader,
        write_artifacts: ArtifactWriteCallback,
        prompt_budgets: PromptBudgets,
        clock: Clock,
        started_at: datetime,
    ) -> None:
        self._decider = decider
        self._executor = executor
        self._load_existing_versions = load_existing_versions
        self._write_artifacts = write_artifacts
        self._prompt_budgets = prompt_budgets
        self._clock = clock
        self._started_at = started_at

    async def load(self, state: DiagnosisState) -> dict[str, Any]:
        """LOAD boundary: read existing artifact versions for this episode
        (idempotent reuse on re-dispatch, CR-REG-46); failures fail the task
        at the runner (frozen-boundary violations are fatal, 3.1.4)."""
        update: dict[str, Any] = {"phase": "LOAD"}
        update.update(await self._load_existing_versions(state))
        return update

    async def decide(self, state: DiagnosisState) -> dict[str, Any]:
        """DECIDE boundary: one structured decision; a parse failure re-asks
        once with the error text (CR-REG-13), a second failure records
        ``unknown_action`` and routes to fallback via ``action=None``."""
        messages = build_decision_messages(
            state, budgets=self._prompt_budgets, now=self._clock.now()
        )
        payload = await self._decider.decide(
            system=messages.system,
            user=messages.user,
            schema_name="DiagnosisDecision",
            schema=DECISION_SCHEMA,
        )
        llm_calls = state.llm_calls + 1
        action, error = parse_action(payload)
        if action is None:
            retry_user = (
                f"{messages.user}\n\n[PARSE_ERROR] Your previous reply failed "
                f"validation: {error}. Reply again with exactly one JSON object."
            )
            payload = await self._decider.decide(
                system=messages.system,
                user=retry_user,
                schema_name="DiagnosisDecision",
                schema=DECISION_SCHEMA,
            )
            llm_calls += 1
            action, retry_error = parse_action(payload)
            if action is None:
                error = retry_error
        update: dict[str, Any] = {"phase": "DECIDE", "llm_calls": llm_calls}
        if action is None:
            update["action"] = None
            update["last_error"] = f"unknown_action: {error}"
        else:
            update["action"] = action
            update["last_error"] = None
        return update

    async def run_tool(self, state: DiagnosisState) -> dict[str, Any]:
        """TOOL boundary: execute the current action through the executor
        port; one action's failure isolates to ``last_error`` and the loop
        continues (never crashes the graph, CR-TASK-09 discipline)."""
        action = state.action
        if action is None:  # routing guarantees an action; defensive only
            return {"phase": "DECIDE", "action": None}
        update: dict[str, Any] = {
            "phase": "DECIDE",
            "tool_rounds": state.tool_rounds + 1,
            "action": None,
        }
        ok = True
        try:
            update.update(await self._executor.execute(state, action))
        except (RetryableError, FatalError) as exc:
            ok = False
            update["last_error"] = f"{exc.code.value}: {exc.message}"
        update["executed_actions"] = [
            ExecutedAction(
                round=state.tool_rounds + 1,
                type=action.type,
                brief=_brief(action),
                ok=ok,
            )
        ]
        return update

    async def fallback(self, state: DiagnosisState) -> dict[str, Any]:
        """Explicit fallback for unknown/malformed actions: count the miss;
        the router decides between re-asking and the partial outcome."""
        return {"no_progress": state.no_progress + 1}

    async def write(self, state: DiagnosisState) -> dict[str, Any]:
        """WRITE boundary: dual-artifact write/registration orchestration
        (T13); DONE is assigned by the runner after the graph ends."""
        update = await self._write_artifacts(state)
        return {"phase": "WRITE", **update}

    async def finalize_partial(self, state: DiagnosisState) -> dict[str, Any]:
        """Budget/no-progress exhaustion: an explicit incomplete outcome with
        collected unknowns -- never a fabricated conclusion (CR-DIAG-14)."""
        unknowns: list[str] = []
        if state.last_error:
            unknowns.append(f"最近错误未解决: {state.last_error}")
        if state.knowledge.called and state.knowledge.error:
            unknowns.append(f"知识调用失败: {state.knowledge.error}")
        if not state.timeline_records:
            unknowns.append("时间轴无记录，未能确认相关监控点与应用事件")
        if state.search_hits:
            unknowns.append(
                "已检索到历史故障但未能完成交叉验证: "
                + ", ".join(hit.fd_id for hit in state.search_hits[-3:])
            )
        report = DiagnosisReport(
            summary="诊断未完成：预算耗尽，以下事项未确认（非最终结论）。",
            observations=[],
            hypotheses=[],
            unknowns=unknowns or ["预算在得出结论前耗尽"],
            next_steps=["提高预算或在恢复窗口内重新派发诊断任务"],
        )
        return {"phase": "DONE", "partial": True, "report": report, "action": None}

    @staticmethod
    def reask_error_code() -> ErrorCode:
        """Stable code surfaced when both decision attempts fail parsing."""
        return ErrorCode.MODEL_PARSE_ERROR