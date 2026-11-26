"""cognitive schema 的异步连接池（CR-REG-38，CR-DATA-03）。

连接池以关闭状态构建（``open=False``），并在异步 lifespan 内打开，
使其后台工作协程挂接到正在运行的事件循环。凭据只来自
``DATABASE_URL_COGNITIVE``——一个仅限 cognitive schema 的专用账号，
绝不使用 Go 系统账号（CR-DATA-01/03）。连接池完全异步：每条异步
路由/worker 都经由它访问 Postgres。
"""

from __future__ import annotations

from psycopg_pool import AsyncConnectionPool

from alert_agent.config import Settings


def build_pool(settings: Settings) -> AsyncConnectionPool:
    """创建处于关闭状态的连接池；在 lifespan 内调用 ``await pool.open()``。"""
    return AsyncConnectionPool(
        conninfo=settings.shared.database_url_cognitive,
        min_size=settings.shared.pool_min_size,
        max_size=settings.shared.pool_max_size,
        name="alert-agent-runtime-cognitive",
        open=False,
    )