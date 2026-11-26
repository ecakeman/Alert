"""诊断用例端口（design 3.1，§7.3）。

``LlmDecider`` 是由 LLM 适配器（T11）实现的结构化决策端口；
``ToolExecutor`` 是由诊断工具批次（T13）实现的动作执行端口。端口只涉及
领域模型（CR-LAY-07）；此处不含任何传输或持久化技术导入（CR-LAY-02）。
"""

from __future__ import annotations

from typing import Any, Protocol

from alert_agent.domain.diagnosis.actions import Action
from alert_agent.domain.diagnosis.state import DiagnosisState


class LlmDecider(Protocol):
    """带 schema 约束与显式低温度的结构化 JSON 决策/报告生成
    （CR-DIAG-08/09）；解析重试保留在实现内部，绝不静默
    （CR-REG-13）。"""

    async def decide(
        self,
        *,
        system: str,
        user: str,
        schema_name: str,
        schema: dict[str, Any],
    ) -> dict[str, Any]: ...


class ToolExecutor(Protocol):
    """对当前状态执行一个已校验的动作并返回增量状态字典
    （CR-DIAG-02，CR-REG-02）；失败被归类为 RetryableError/FatalError
    并浮出，绝不吞掉（CR-REG-47）。"""

    async def execute(self, state: DiagnosisState, action: Action) -> dict[str, Any]: ...