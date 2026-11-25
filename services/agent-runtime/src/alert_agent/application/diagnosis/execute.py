"""ExecuteDiagnosis 用例：DIAGNOSIS 任务对应的 TaskExecutor 端口实现
（T13，design 4.2/4.3，CR-DIAG-*）。

一次调用覆盖整个诊断任务：started 回调、任务上下文一致性检查、在冻结边界
之上构建图、有界循环执行、双产物写入/注册（经 WRITE 节点）以及结果回调。
失败在 ``report_failure`` 之后以领域错误的形式浮出（携带
retryable/code/message，CR-TASK-12/13）；心跳阈值取消引发的
CancelledError 原样向上传播（CR-TASK-06/07）。
"""

from __future__ import annotations

import logging

from alert_agent.application.diagnosis.artifacts import DiagnosisArtifactOrchestrator
from alert_agent.application.diagnosis.graph import build_diagnosis_graph
from alert_agent.application.diagnosis.nodes import DiagnosisNodes
from alert_agent.application.diagnosis.ports import LlmDecider, ToolExecutor
from alert_agent.application.diagnosis.prompt import PromptBudgets
from alert_agent.application.diagnosis.routing import DiagnosisRouter
from alert_agent.application.shared.ports import ArtifactWriter, Clock
from alert_agent.application.task.executor import TaskExecutor
from alert_agent.application.task.ports import (
    ArtifactStore,
    TaskFailureReport,
    TaskGateway,
    TaskSubmission,
)
from alert_agent.config import Settings
from alert_agent.config.settings import DiagnosisSettings
from alert_agent.domain.diagnosis.budget import DiagnosisBudget
from alert_agent.domain.diagnosis.state import DiagnosisState
from alert_agent.domain.shared import ErrorCode, FatalError, RetryableError

LOGGER = logging.getLogger(__name__)


def prompt_budgets_from(settings: DiagnosisSettings) -> PromptBudgets:
    """唯一来源的提示词预算（CR-REG-05；无重复常量）。"""
    return PromptBudgets(
        timeline_chars=settings.prompt_timeline_budget_chars,
        search_chars=settings.prompt_search_budget_chars,
        knowledge_chars=settings.prompt_knowledge_budget_chars,
        mcp_chars=settings.prompt_mcp_budget_chars,
        summary_items=settings.prompt_summary_items,
        actions_items=settings.prompt_actions_items,
        last_error_chars=settings.prompt_last_error_max_chars,
    )


class ExecuteDiagnosis(TaskExecutor):
    """DIAGNOSIS 任务执行器；由于路由器/节点携带该任务的时钟与开始时间，
    图按任务逐一编译。"""

    def __init__(
        self,
        *,
        gateway: TaskGateway,
        artifact_store: ArtifactStore,
        artifact_writer: ArtifactWriter,
        decider: LlmDecider,
        tool_executor: ToolExecutor,
        settings: Settings,
        clock: Clock,
    ) -> None:
        self._gateway = gateway
        self._artifacts = DiagnosisArtifactOrchestrator(
            store=artifact_store, writer=artifact_writer, gateway=gateway
        )
        self._decider = decider
        self._tool_executor = tool_executor
        self._settings = settings
        self._clock = clock

    async def execute(self, submission: TaskSubmission) -> None:
        identity = submission.identity
        if identity.task_type != "DIAGNOSIS":
            raise FatalError(
                ErrorCode.INPUT_INVALID,
                f"ExecuteDiagnosis received a {identity.task_type} task",
            )
        await self._gateway.report_started(identity)
        context = await self._gateway.get_task_context(identity)
        boundary = submission.boundary
        if (
            context.input_version_from != boundary.input_version_from
            or context.input_version_to != boundary.input_version_to
            or context.task_type != identity.task_type
        ):
            raise FatalError(
                ErrorCode.BOUNDARY_OUT_OF_RANGE,
                "GetTaskContext diverges from the frozen boundary of the submission",
            )

        diagnosis_settings = self._settings.diagnosis
        budget = DiagnosisBudget(
            max_tool_rounds=diagnosis_settings.diagnosis_max_tool_rounds,
            max_llm_calls=diagnosis_settings.diagnosis_max_llm_calls,
            deadline_ratio=diagnosis_settings.diagnosis_deadline_ratio,
            max_no_progress=diagnosis_settings.diagnosis_max_no_progress,
        )
        started_at = self._clock.now()
        initial = DiagnosisState(
            identity=identity,
            # 契约不携带 service_name；id 即为诚实的展示值
            # （不编造名称）。
            service_id=submission.service_id,
            service_name=submission.service_id,
            boundary=boundary,
            phase="LOAD",
            budget=budget,
        )
        nodes = DiagnosisNodes(
            decider=self._decider,
            executor=self._tool_executor,
            load_existing_versions=self._artifacts.load_existing_versions,
            write_artifacts=self._artifacts.write,
            prompt_budgets=prompt_budgets_from(diagnosis_settings),
            clock=self._clock,
            started_at=started_at,
        )
        router = DiagnosisRouter(self._clock, started_at)
        graph = build_diagnosis_graph(nodes, router)
        # 递归上限由预算推导（无魔法常量）：最坏链路为每轮
        # (decide + tool)，外加 load/write/finalize 的余量。
        recursion_limit = diagnosis_settings.diagnosis_max_tool_rounds * 3 + 8
        try:
            final = await graph.ainvoke(
                initial.model_dump(), config={"recursion_limit": recursion_limit}
            )
        except (RetryableError, FatalError) as exc:
            # report_failure 在此处发出（worker 只写回执）；
            # Go 侧的栅栏语义使重复回放具有幂等性（CR-TASK-12/13）。
            await self._gateway.report_failure(
                TaskFailureReport(
                    identity=submission.identity,
                    retryable=isinstance(exc, RetryableError),
                    error_code=exc.code.value,
                    error_message=exc.message,
                )
            )
            raise
        final_state = DiagnosisState.model_validate(final)
        LOGGER.info(
            "diagnosis task %s gen %d finished phase=%s partial=%s",
            identity.task_id,
            identity.generation,
            final_state.phase,
            final_state.partial,
        )
        # partial=true 仍是一个已完成的任务：显式的不完整报告已注册为
        # DIAGNOSIS_REPORT（CR-DIAG-14）。
        await self._gateway.report_result(identity)