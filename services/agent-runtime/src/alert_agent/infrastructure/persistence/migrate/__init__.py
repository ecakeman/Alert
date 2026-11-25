"""cognitive schema 迁移适配器：仅向前执行器（D6，CR-REG-37/49）。"""

from __future__ import annotations

from alert_agent.infrastructure.persistence.migrate.runner import (
    MigrationError,
    MigrationRecord,
    run_migrations,
)

__all__ = ["MigrationError", "MigrationRecord", "run_migrations"]