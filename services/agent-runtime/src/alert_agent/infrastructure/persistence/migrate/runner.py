"""cognitive schema 的仅向前迁移执行器（design 6.1，D6）。

每个迁移文件整体执行，处于单个事务中，走 libpq 简单查询协议（psycopg3
在未绑定参数时原样发送脚本）——禁止按 ``;`` 切分（CR-REG-37，RAG D1）。
``version + name + SHA-256 校验和`` 记录在 ``cognitive.schema_migrations``；
重放已应用的版本是无操作，校验和不一致则显式报错，因为 schema 是仅向前的
（D6）。连接仅使用 ``DATABASE_URL_COGNITIVE``（CR-DATA-01）：账号限定于
cognitive schema，且不得持有任何 system schema 权限（CR-DATA-03）。
"""

from __future__ import annotations

import hashlib
import re
from dataclasses import dataclass
from pathlib import Path

import psycopg

# 引导语句在任何迁移文件之前执行：保证记录表存在，以便第一条已应用的
# 迁移可以被记账。它们与 0001/0005 一致且幂等。
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
    """迁移失败；消息中写明文件与原因，绝不包含 DSN 值。"""


@dataclass(frozen=True)
class MigrationRecord:
    """本次运行中一个迁移文件的处理结果。"""

    version: int
    name: str
    checksum: str
    applied: bool  # False = 已应用且校验一致，跳过（无操作）


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
        # 整文件执行：无参数绑定 -> 简单查询协议 ->
        # 文件中的每条语句都在这一个事务内运行。
        # 脚本绝不按 ';' 切分（CR-REG-37）。
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
    """按文件名顺序应用待执行的 cognitive 迁移；返回每个文件的结果。

    已应用且校验和一致的文件被跳过；校验和不一致则报错。任何失败只回滚
    当前文件（先前的文件保持已提交），并抛出脱敏后的 ``MigrationError``。
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