"""Foundational ports shared by use cases (design 5.1, CR-LAY-06).

``Clock`` stamps receipts and retries with timezone-aware UTC instants;
``IdGen`` mints application-side identifiers (artifact ids, call ids) so
id formats never depend on the database. Real implementations live in
bootstrap wiring; use cases only see these protocols.
"""

from __future__ import annotations

from datetime import datetime
from typing import Protocol


class Clock(Protocol):
    def now(self) -> datetime:
        """Current timezone-aware instant."""
        ...


class IdGen(Protocol):
    def new_id(self, prefix: str) -> str:
        """A fresh unique identifier carrying the given semantic prefix."""
        ...