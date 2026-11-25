"""归档领域模型（M6）：故障搜索索引与故障文档。"""

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