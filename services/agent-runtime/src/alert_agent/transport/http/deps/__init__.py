"""HTTP 传输层依赖（T18+）：内部令牌鉴权及相关依赖。"""

from alert_agent.transport.http.deps.security import require_internal_token

__all__ = ["require_internal_token"]