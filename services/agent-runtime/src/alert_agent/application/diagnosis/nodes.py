"""诊断循环节点实现（design 3.1.4/3.1.5）。

节点只返回增量字典（CR-DIAG-02，CR-REG-02）；phase 恰好在它所属的边界处
被赋值（CR-DIAG-03）。既有版本读取与产物写入是由执行批次（T13）注入的
可调用对象，因此本模块不包含任何 IO（CR-LAY-02）。每条失败路径都会记录
可读的 ``last_error``；没有任何内容被吞掉（CR-REG-47）。
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
    """接入编译后状态图的节点集合。"""

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
        """LOAD 边界：读取该 episode 的既有产物版本
        （重新派发时幂等复用，CR-REG-46）；失败则由运行器判定任务失败
        （冻结边界违规是致命的，3.1.4）。"""
        update: dict[str, Any] = {"phase": "LOAD"}
        update.update(await self._load_existing_versions(state))
        return update

    async def decide(self, state: DiagnosisState) -> dict[str, Any]:
        """DECIDE 边界：一次结构化决策；解析失败时携带错误文本追问一次
        （CR-REG-13），第二次失败记录 ``unknown_action`` 并通过
        ``action=None`` 路由到 fallback。"""
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
        """TOOL 边界：通过执行器端口执行当前动作；单个动作的失败被隔离到
        ``last_error`` 且循环继续（绝不使图崩溃，CR-TASK-09 纪律）。"""
        action = state.action
        if action is None:  # 路由保证存在动作；仅作防御
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
        """未知/畸形动作的显式回退：计数该次失配；
        由路由器决定追问还是输出部分结果。"""
        return {"no_progress": state.no_progress + 1}

    async def write(self, state: DiagnosisState) -> dict[str, Any]:
        """WRITE 边界：双产物写入/注册编排（T13）；DONE 由运行器在图结束
        之后赋值。"""
        update = await self._write_artifacts(state)
        return {"phase": "WRITE", **update}

    async def finalize_partial(self, state: DiagnosisState) -> dict[str, Any]:
        """预算/无进展耗尽：带有已收集 unknowns 的显式不完整结果——
        绝不编造结论（CR-DIAG-14）。"""
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
        """两次决策尝试都解析失败时浮出的稳定错误码。"""
        return ErrorCode.MODEL_PARSE_ERROR