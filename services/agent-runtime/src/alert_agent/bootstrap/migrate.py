"""认知迁移命令入口（``python -m alert_agent.bootstrap.migrate``）。

在 ``DATABASE_URL_COGNITIVE`` 上对 ``services/agent-runtime/migrations`` 执行
只前进的迁移运行器（T04，CR-DATA-01/02）。配置失败与迁移失败均以非零退出码
结束并输出脱敏报告；每个文件报告为已应用或已跳过（三态、如实报告）。
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