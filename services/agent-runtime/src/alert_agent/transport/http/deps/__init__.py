"""HTTP transport dependencies (T18+): internal-token auth and friends."""

from alert_agent.transport.http.deps.security import require_internal_token

__all__ = ["require_internal_token"]