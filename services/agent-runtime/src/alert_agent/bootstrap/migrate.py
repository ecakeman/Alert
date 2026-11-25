"""Cognitive migration command entry (``python -m alert_agent.bootstrap.migrate``).

Runs the forward-only runner (T04) over ``services/agent-runtime/migrations``
against ``DATABASE_URL_COGNITIVE`` (CR-DATA-01/02). Configuration failures and
migration failures both exit non-zero with a desensitised report; each file is
reported as applied or skipped (three-state, honest reporting).
"""

from __future__ import annotations

import sys
from pathlib import Path

from alert_agent.config import ConfigError, load_settings
from alert_agent.infrastructure.persistence.migrate.runner import MigrationError, run_migrations

# bootstrap/migrate.py -> bootstrap -> alert_agent -> src -> agent-runtime
_MIGRATIONS_DIR = Path(__file__).resolve().parents[3] / "migrations"


def main() -> int:
    try:
        settings = load_settings()
    except ConfigError as exc:
        print(str(exc), file=sys.stderr)
        return 1
    try:
        records = run_migrations(settings.shared.database_url_cognitive, _MIGRATIONS_DIR)
    except MigrationError as exc:
        print(f"cognitive migration failed: {exc}", file=sys.stderr)
        return 1
    except OSError as exc:
        print(f"cognitive migration failed: {exc}", file=sys.stderr)
        return 1
    for record in records:
        state = "applied" if record.applied else "skipped (already applied, checksum verified)"
        print(f"{record.version:04d}_{record.name}: {state}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())