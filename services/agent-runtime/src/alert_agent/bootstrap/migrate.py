"""Cognitive migration command entry (``python -m alert_agent.bootstrap.migrate``).

The forward-only runner is implemented by the migration batch (T04); until
then this entry validates configuration and honestly reports that nothing
was executed (three-state discipline, CR-DISC-11).
"""

from __future__ import annotations

import sys

from alert_agent.config import ConfigError, load_settings


def main() -> int:
    try:
        load_settings()
    except ConfigError as exc:
        print(str(exc), file=sys.stderr)
        return 1
    print("cognitive migration runner lands in T04; nothing executed", file=sys.stderr)
    return 2


if __name__ == "__main__":
    raise SystemExit(main())