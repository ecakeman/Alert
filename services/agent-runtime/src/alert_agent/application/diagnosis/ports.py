"""Diagnosis use-case ports (design 3.1, §7.3).

``LlmDecider`` is the structured-decision port implemented by the LLM adapter
(T11); ``ToolExecutor`` is the action-execution port implemented by the
diagnosis tool batch (T13). Ports speak domain models only (CR-LAY-07); no
transport or persistence technology imports here (CR-LAY-02).
"""

from __future__ import annotations

from typing import Any, Protocol

from alert_agent.domain.diagnosis.actions import Action
from alert_agent.domain.diagnosis.state import DiagnosisState


class LlmDecider(Protocol):
    """Structured JSON decision/report generation with schema constraint and
    explicit low temperature (CR-DIAG-08/09); parse retries stay inside the
    implementation, never silent (CR-REG-13)."""

    async def decide(
        self,
        *,
        system: str,
        user: str,
        schema_name: str,
        schema: dict[str, Any],
    ) -> dict[str, Any]: ...


class ToolExecutor(Protocol):
    """Executes one validated action against the current state and returns an
    incremental state dict (CR-DIAG-02, CR-REG-02); failures are classified as
    RetryableError/FatalError and surfaced, never swallowed (CR-REG-47)."""

    async def execute(self, state: DiagnosisState, action: Action) -> dict[str, Any]: ...