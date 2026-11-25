"""知识智能体调用端口（design 4.5.2，CR-KNOW-01~08）。

A2A 客户端（T22）实现此端口；诊断循环（T13）只依赖该端口。结果在进入
诊断状态前必须通过 ``list[KnowledgeHit]`` 校验（CR-KNOW-07）；非
COMPLETED 的结果会抛出携带 code 与安全消息的可诊断错误（CR-KNOW-08）。
"""

from __future__ import annotations

from typing import Protocol

from alert_agent.domain.diagnosis.records import FdHit
from alert_agent.domain.knowledge.models import InvocationV1, KnowledgeHit


class KnowledgeInvoker(Protocol):
    async def invoke(self, invocation: InvocationV1) -> list[KnowledgeHit]: ...


class FaultDocSearcher(Protocol):
    """在认知存储内进行历史故障文档检索
    （CR-DIAG-12，由归档批次 T14 实现）。

    实现优先对同一服务排序并提供跨服务回退，融合向量与全文两路，
    返回通过 ``FdHit`` 校验的可追溯 ``fd_id`` 命中。
    """

    async def search(self, service_id: str, query: str, *, top_k: int = 5) -> list[FdHit]: ...