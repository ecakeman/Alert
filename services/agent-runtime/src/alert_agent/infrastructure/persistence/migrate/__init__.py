"""Cognitive schema migration adapters: forward-only runner (D6, CR-REG-37/49)."""

from __future__ import annotations

from alert_agent.infrastructure.persistence.migrate.runner import (
    MigrationError,
    MigrationRecord,
    run_migrations,
)

__all__ = ["MigrationError", "MigrationRecord", "run_migrations"]