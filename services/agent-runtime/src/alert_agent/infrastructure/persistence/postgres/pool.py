"""Async connection pool for the cognitive schema (CR-REG-38, CR-DATA-03).

The pool is built closed (``open=False``) and opened inside the async lifespan
so its background workers attach to the running event loop. Credentials come
only from ``DATABASE_URL_COGNITIVE`` -- a dedicated account restricted to the
cognitive schema, never the Go system account (CR-DATA-01/03). The pool is
fully asynchronous: every async route/worker reaches Postgres through it.
"""

from __future__ import annotations

from psycopg_pool import AsyncConnectionPool

from alert_agent.config import Settings


def build_pool(settings: Settings) -> AsyncConnectionPool:
    """Create the closed pool; call ``await pool.open()`` inside the lifespan."""
    return AsyncConnectionPool(
        conninfo=settings.shared.database_url_cognitive,
        min_size=settings.shared.pool_min_size,
        max_size=settings.shared.pool_max_size,
        name="alert-agent-runtime-cognitive",
        open=False,
    )