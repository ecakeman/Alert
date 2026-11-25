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

class ArtifactWriter(Protocol):
    """Artifact file writer port (design 4.6, D5, CR-GW-09/09a).

    Content lives only in files; databases and callbacks carry the
    storage_key address and hashes, never the body (CR-DATA-04).
    """

    async def write(self, storage_key: str, content: bytes) -> str:
        """Atomically persist content under the storage_key.

        Writes are atomic (temp file + rename) and the returned sha-256 hex
        digest is the single hash computation shared by the cognitive
        registration and the Go callback (CR-LAY-10, CR-GW-09a).
        """
        ...