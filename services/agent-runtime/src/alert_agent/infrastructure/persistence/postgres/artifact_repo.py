"""ArtifactStore implementation on cognitive.episode_artifacts (design 4.6,
CR-DIAG-19, CR-REG-46).

UNIQUE(episode_id, artifact_type, artifact_version) is the optimistic-lock
basis: INSERT ... ON CONFLICT DO NOTHING first, then compare the existing row
-- same generation + same hash is an idempotent hit (Q7), an existing row of a
newer generation is never overwritten (older generations cannot register,
CR-REG-46), anything else is an ``ARTIFACT_VERSION_CONFLICT``. There is no
artifact content column (D5); the storage_key addresses the file body.
"""

from __future__ import annotations

from dataclasses import dataclass

from psycopg.rows import class_row
from psycopg_pool import AsyncConnectionPool

from alert_agent.application.shared.ports import IdGen
from alert_agent.application.task.ports import ArtifactRecord, ArtifactRegistration
from alert_agent.domain.shared import ErrorCode, FatalError, RetryableError

_INSERT_ARTIFACT = """
INSERT INTO cognitive.episode_artifacts
    (artifact_id, task_id, generation, episode_id, artifact_type, artifact_version,
     content_hash, content_type, storage_key)
VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s)
ON CONFLICT (episode_id, artifact_type, artifact_version) DO NOTHING
RETURNING artifact_id, task_id, generation, episode_id, artifact_type,
          artifact_version, content_hash, content_type, storage_key
"""

_SELECT_ARTIFACT = """
SELECT artifact_id, task_id, generation, episode_id, artifact_type,
       artifact_version, content_hash, content_type, storage_key
FROM cognitive.episode_artifacts
WHERE episode_id = %s AND artifact_type = %s AND artifact_version = %s
"""

_SELECT_LATEST_VERSION = """
SELECT max(artifact_version) FROM cognitive.episode_artifacts
WHERE episode_id = %s AND artifact_type = %s
"""


@dataclass(frozen=True, slots=True)
class ArtifactRow:
    """Projection of cognitive.episode_artifacts used by the artifact repo."""

    artifact_id: str
    task_id: str
    generation: int
    episode_id: str
    artifact_type: str
    artifact_version: int
    content_hash: str
    content_type: str
    storage_key: str


def _to_record(row: ArtifactRow) -> ArtifactRecord:
    return ArtifactRecord(
        artifact_id=row.artifact_id,
        task_id=row.task_id,
        generation=row.generation,
        episode_id=row.episode_id,
        artifact_type=row.artifact_type,
        artifact_version=row.artifact_version,
        content_hash=row.content_hash,
        content_type=row.content_type,
        storage_key=row.storage_key,
    )


class PostgresArtifactStore:
    """ArtifactStore port implementation on cognitive.episode_artifacts."""

    def __init__(self, pool: AsyncConnectionPool, ids: IdGen) -> None:
        self._pool = pool
        self._ids = ids

    async def register(self, registration: ArtifactRegistration) -> ArtifactRecord:
        async with self._pool.connection() as connection:
            cursor = await connection.execute(
                _INSERT_ARTIFACT,
                (
                    self._ids.new_id("artifact"),
                    registration.task_id,
                    registration.generation,
                    registration.episode_id,
                    registration.artifact_type,
                    registration.artifact_version,
                    registration.content_hash,
                    registration.content_type,
                    registration.storage_key,

                ),
            )
            row = await cursor.fetchone()
            if row is not None:
                return ArtifactRecord(
                    artifact_id=row[0],
                    task_id=row[1],
                    generation=row[2],
                    episode_id=row[3],
                    artifact_type=row[4],
                    artifact_version=row[5],
                    content_hash=row[6],
                    content_type=row[7],
                    storage_key=row[8],
                )
            existing = await self._fetch(
                connection, registration.episode_id, registration.artifact_type, registration.artifact_version
            )
            if existing is None:
                # Unreachable unless the concurrent insert that won the UNIQUE
                # race aborted between ON CONFLICT and this read; retrying is safe.
                raise RetryableError(
                    ErrorCode.ARTIFACT_VERSION_CONFLICT,
                    "concurrent artifact insert rolled back; retry the registration",
                )
            if existing.generation > registration.generation:
                raise FatalError(
                    ErrorCode.ARTIFACT_VERSION_CONFLICT,
                    f"artifact version {registration.artifact_version} is held by newer"
                    f" generation {existing.generation}; older generation must not overwrite",
                )
            if (
                existing.generation == registration.generation
                and existing.content_hash == registration.content_hash
            ):
                # Same version + same hash: idempotent reuse (CR-REG-46, Q7).
                return _to_record(existing)
            raise FatalError(
                ErrorCode.ARTIFACT_VERSION_CONFLICT,
                f"artifact version {registration.artifact_version} is already registered"
                " with different content",
            )

    async def latest_version(self, episode_id: str, artifact_type: str) -> int:
        async with self._pool.connection() as connection:
            cursor = await connection.execute(
                _SELECT_LATEST_VERSION, (episode_id, artifact_type)
            )
            row = await cursor.fetchone()
            return int(row[0]) if row is not None and row[0] is not None else 0

    async def find(
        self, episode_id: str, artifact_type: str, artifact_version: int
    ) -> ArtifactRecord | None:
        async with (
            self._pool.connection() as connection,
            connection.cursor(row_factory=class_row(ArtifactRow)) as cursor,
        ):
            await cursor.execute(
                _SELECT_ARTIFACT, (episode_id, artifact_type, artifact_version)
            )
            row = await cursor.fetchone()
        return _to_record(row) if row is not None else None

    @staticmethod
    async def _fetch(
        connection, episode_id: str, artifact_type: str, artifact_version: int
    ) -> ArtifactRow | None:
        async with connection.cursor(row_factory=class_row(ArtifactRow)) as cursor:
            await cursor.execute(
                _SELECT_ARTIFACT, (episode_id, artifact_type, artifact_version)
            )
            return await cursor.fetchone()