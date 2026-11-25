"""动作判别联合类型（design 3.1.2，CR-DIAG-04/05/06）。

五种动作类型，``extra="forbid"`` 使格式错误的模型输出
在校验阶段失败并路由到显式的回退节点（CR-DIAG-05）。
判别符为 ``type``；``parse_action``（应用层，T12）在解包后将原始
JSON 交给 ``Action.model_validate``/TypeAdapter 处理。
"""

from __future__ import annotations

from typing import Annotated, Any, Literal

from pydantic import BaseModel, ConfigDict, Field

from alert_agent.domain.diagnosis.report import DiagnosisReport


class ActionBase(BaseModel):
    model_config = ConfigDict(extra="forbid")


class QueryTimelineAction(ActionBase):
    """有意不设参数：时间范围始终来自冻结边界，且 Go 侧
    会拒绝越界的读取请求（CR-DIAG-10）。"""

    type: Literal["query_timeline"]


class SearchFaultsAction(ActionBase):
    type: Literal["search_faults"]
    query: str = Field(min_length=1)


class InvokeKnowledgeAction(ActionBase):
    type: Literal["invoke_knowledge_agent"]
    query: str = Field(min_length=1)
    top_k: int = Field(default=8, ge=1, le=20)


class CallMcpToolAction(ActionBase):
    """``name`` 在执行前会对照实时的 tools/list 目录进行
    校验；``arguments`` 必须是显式的 JSON 对象（CR-DIAG-06）。"""

    type: Literal["call_mcp_tool"]
    name: str = Field(min_length=1)
    arguments: dict[str, Any]


class FinishAction(ActionBase):
    type: Literal["finish"]
    report: DiagnosisReport


Action = Annotated[
    QueryTimelineAction
    | SearchFaultsAction
    | InvokeKnowledgeAction
    | CallMcpToolAction
    | FinishAction,
    Field(discriminator="type"),
]