"""诊断领域模型测试（CR-DIAG-01~06，7.2.6）：判别联合、宽松预处理与预算终止。"""

import pytest
from pydantic import ValidationError, TypeAdapter

from alert_agent.application.diagnosis.routing import budget_exhausted
from alert_agent.domain.diagnosis.actions import (
    QueryTimelineAction,
    SearchFaultsAction,
)
from alert_agent.domain.diagnosis.budget import DiagnosisBudget
from alert_agent.domain.diagnosis.report import DiagnosisReport


def _adapter():
    from alert_agent.domain.diagnosis.actions import Action as _Action

    return TypeAdapter(_Action)


def test_action_union_validates_and_discriminates() -> None:
    adapter = _adapter()
    action = adapter.validate_python({"type": "query_timeline"})
    assert isinstance(action, QueryTimelineAction)
    assert isinstance(
        adapter.validate_python({"type": "search_faults", "query": "gateway 5xx"}),
        SearchFaultsAction,
    )


def test_action_union_rejects_unknown_type_and_extra_fields() -> None:
    adapter = _adapter()
    with pytest.raises(ValidationError):
        adapter.validate_python({"type": "no_such_type"})
    with pytest.raises(ValidationError):
        adapter.validate_python({"type": "search_faults", "query": "x", "extra": 1})
    with pytest.raises(ValidationError):
        adapter.validate_python({"type": "search_faults", "query": ""})


def test_report_loose_scalar_becomes_list_strict_fields_stay() -> None:
    report = DiagnosisReport.model_validate(
        {"summary": "ok", "observations": "单一现象"}
    )
    assert report.observations == ["单一现象"]
    assert report.hypotheses == []
    with pytest.raises(ValidationError):
        DiagnosisReport.model_validate({"summary": ""})


def test_budget_exhausted_is_single_gate() -> None:
    from datetime import UTC, datetime, timedelta

    from alert_agent.domain.diagnosis.state import DiagnosisState
    from alert_agent.domain.shared import FrozenBoundary, TaskIdentity

    started = datetime.now(UTC)
    identity = TaskIdentity(
        task_id="t", episode_id="e", task_type="DIAGNOSIS", generation=1, lease_token="L"
    )
    boundary = FrozenBoundary(
        input_version_from=1,
        input_version_to=2,
        monitor_watermark=0,
        app_event_watermark=0,
        deadline_at=started + timedelta(hours=1),
    )
    state = DiagnosisState(
        identity=identity,
        service_id="svc",
        service_name="svc",
        boundary=boundary,
        phase="DECIDE",
        budget=DiagnosisBudget(max_tool_rounds=2),
    )
    # 预算终止唯一入口：application/diagnosis/routing.budget_exhausted（3.1.4）。
    assert not budget_exhausted(state, now=started, started_at=started)
    assert budget_exhausted(
        state.model_copy(update={"tool_rounds": 2}), now=started, started_at=started
    )
    assert budget_exhausted(state, now=started + timedelta(hours=2), started_at=started)
