"""Archive domain models (M6): fault search index and fault document."""

from alert_agent.domain.archive.index import (
    FaultDocument,
    FaultSearchIndex,
    content_hash_of,
    fd_id_of,
)

__all__ = [
    "FaultDocument",
    "FaultSearchIndex",
    "content_hash_of",
    "fd_id_of",
]