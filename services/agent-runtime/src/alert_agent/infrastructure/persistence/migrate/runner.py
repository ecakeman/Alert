"""Forward-only migration runner for the cognitive schema (design 6.1, D6).

Each migration file is executed WHOLE, in a single transaction, through the
libpq simple query protocol (psycopg3 sends the script unsplit when no
parameters are bound) -- splitting on ``;`` is forbidden (CR-REG-37, RAG D1).
``version + name + SHA-256 checksum`` are recorded in
``cognitive.schema_migrations``; replaying an applied version is a no-op, and
a checksum mismatch fails loudly because the schema is forward-only (D6).
The connection uses ``DATABASE_URL_COGNITIVE`` only (CR-DATA-01): the account
is restricted to the cognitive schema and must hold no system-schema rights
(CR-DATA-03).
"""

from __future__ import annotations

import hashlib
import re
from dataclasses import dataclass
from pathlib import Path

import psycopg

# Bootstrap statements run before any file: they guarantee the record table
# exists so the first applied migration can be journaled. They mirror 0001/0005
# and are idempotent.
_BOOTSTRAP_STATEMENTS: tuple[str, ...] = (
    "CREATE SCHEMA IF NOT EXISTS cognitive",
    (
        "CREATE TABLE IF NOT EXISTS cognitive.schema_migrations ("
        " version integer NOT NULL,"
        " name text NOT NULL,"
        " checksum text NOT NULL,"
        " applied_at timestamptz NOT NULL DEFAULT now(),"
        " CONSTRAINT schema_migrations_pk PRIMARY KEY (version))"
    ),
)

_MIGRATION_FILE = re.compile(r"^(\d{4})_([a-z0-9_]+)\.sql$")


class MigrationError(Exception):
    """Migration failure; the message names the file and reason, never DSN values."""


@dataclass(frozen=True)
class MigrationRecord:
    """Result of one migration file for this run."""

    version: int
    name: str
    checksum: str
    applied: bool  # False = already applied and verified, skipped (no-op)


def _discover(migrations_dir: Path) -> list[tuple[int, str, Path]]:
    if not migrations_dir.is_dir():
        raise MigrationError(f"migrations directory not found: {migrations_dir}")
    entries: list[tuple[int, str, Path]] = []
    for path in sorted(migrations_dir.iterdir()):
        match = _MIGRATION_FILE.match(path.name)
        if match is not None:
            entries.append((int(match.group(1)), match.group(2), path))
    versions = [version for version, _, _ in entries]
    if len(versions) != len(set(versions)):
        raise MigrationError(f"duplicate migration versions in {migrations_dir}")
    if not entries:
        raise MigrationError(f"no migration files found in {migrations_dir}")
    return entries


def _sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def _already_applied(
    cursor: psycopg.Cursor, version: int, checksum: str
) -> bool:
    cursor.execute(
        "SELECT checksum FROM cognitive.schema_migrations WHERE version = %s",
        (version,),
    )
    row = cursor.fetchone()
    if row is None:
        return False
    recorded = row[0]
    if recorded != checksum:
        raise MigrationError(
            f"migration {version:04d} checksum drift: the applied file changed after apply"
            " (forward-only, refusing to re-run)"
        )
    return True


def _apply_whole_file(
    connection: psycopg.Connection,
    cursor: psycopg.Cursor,
    version: int,
    name: str,
    checksum: str,
    path: Path,
) -> None:
    script = path.read_text(encoding="utf-8")
    try:
        # Whole-file execution: no parameter binding -> simple query protocol ->
        # every statement of the file runs inside this one transaction. The
        # script is never split on ';' (CR-REG-37).
        cursor.execute(script)
        cursor.execute(
            "INSERT INTO cognitive.schema_migrations (version, name, checksum)"
            " VALUES (%s, %s, %s) ON CONFLICT (version) DO NOTHING",
            (version, name, checksum),
        )
        connection.commit()
    except psycopg.Error as exc:
        connection.rollback()
        raise MigrationError(f"migration {version:04d}_{name} failed: {_primary(exc)}") from exc


def _primary(exc: psycopg.Error) -> str:
    diag = getattr(exc, "diag", None)
    message = getattr(diag, "message_primary", None)
    return message if message else str(exc)


def run_migrations(dsn: str, migrations_dir: Path) -> list[MigrationRecord]:
    """Apply pending cognitive migrations in filename order; return per-file results.

    Already-applied files with matching checksums are skipped; checksum drift
    raises. Any failure rolls back the current file only (earlier files stay
    committed) and raises ``MigrationError`` with a desensitised message.
    """
    entries = _discover(migrations_dir)
    records: list[MigrationRecord] = []
    try:
        connection = psycopg.connect(dsn)
    except psycopg.Error as exc:
        raise MigrationError(f"cannot connect to the cognitive database: {_primary(exc)}") from exc
    with connection:
        bootstrap_cursor = connection.cursor()
        try:
            for statement in _BOOTSTRAP_STATEMENTS:
                bootstrap_cursor.execute(statement)
            connection.commit()
        except psycopg.Error as exc:
            connection.rollback()
            raise MigrationError(f"migration bootstrap failed: {_primary(exc)}") from exc
        for version, name, path in entries:
            data = path.read_bytes()
            checksum = _sha256(data)
            with connection.cursor() as cursor:
                if _already_applied(cursor, version, checksum):
                    records.append(MigrationRecord(version, name, checksum, applied=False))
                    continue
            _apply_whole_file(connection, connection.cursor(), version, name, checksum, path)
            records.append(MigrationRecord(version, name, checksum, applied=True))
    return records