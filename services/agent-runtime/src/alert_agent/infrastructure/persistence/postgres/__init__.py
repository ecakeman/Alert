"""Postgres persistence adapters for the cognitive schema (psycopg_pool, all async)."""

from __future__ import annotations

from alert_agent.infrastructure.persistence.postgres.pool import build_pool
from alert_agent.infrastructure.persistence.postgres.receipt_repo import PostgresReceiptStore
from alert_agent.infrastructure.persistence.postgres.rows import ReceiptRow

__all__ = ["PostgresReceiptStore", "ReceiptRow", "build_pool"]