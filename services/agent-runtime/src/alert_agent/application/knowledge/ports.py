"""Knowledge-agent invocation port (design 4.5.2, CR-KNOW-01~08).

The A2A client (T22) implements this port; the diagnosis loop (T13) depends
only on the port. Results must validate as ``list[KnowledgeHit]`` before
entering diagnosis state (CR-KNOW-07); non-COMPLETED outcomes raise a
diagnosable error carrying code and safe message (CR-KNOW-08).
"""

from __future__ import annotations

from typing import Protocol

from alert_agent.domain.diagnosis.records import FdHit
from alert_agent.domain.knowledge.models import InvocationV1, KnowledgeHit


class KnowledgeInvoker(Protocol):
    async def invoke(self, invocation: InvocationV1) -> list[KnowledgeHit]: ...


class FaultDocSearcher(Protocol):
    """Historical fault-document retrieval inside the cognitive store
    (CR-DIAG-12, implemented by the archive batch T14).

    Implementations rank the same service first with a cross-service
    fallback, fuse vector and full-text legs and return traceable ``fd_id``
    hits validated as ``FdHit``.
    """

    async def search(self, service_id: str, query: str, *, top_k: int = 5) -> list[FdHit]: ...