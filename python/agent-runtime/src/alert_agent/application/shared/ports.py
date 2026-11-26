"""用例共享的基础端口（design 5.1，CR-LAY-06）。

``Clock`` 用带时区的 UTC 时刻为回执与重试打戳；``IdGen`` 生成应用侧的
标识符（artifact id、call id），使 id 格式永远不依赖数据库。真正的实现
位于 bootstrap 接线中；用例只看到这些协议。
"""

from __future__ import annotations

from datetime import datetime
from typing import Protocol


class Clock(Protocol):
    def now(self) -> datetime:
        """当前的带时区时刻。"""
        ...


class IdGen(Protocol):
    def new_id(self, prefix: str) -> str:
        """携带给定语义前缀的全新唯一标识符。"""
        ...

class ArtifactWriter(Protocol):
    """产物文件写入端口（design 4.6，D5，CR-GW-09/09a）。

    内容只存在于文件中；数据库与回调只携带 storage_key 地址与哈希，
    绝不携带正文（CR-DATA-04）。
    """

    async def write(self, storage_key: str, content: bytes) -> str:
        """将内容原子性地持久化到 storage_key 之下。

        写入是原子性的（临时文件 + 重命名），返回的 sha-256 十六进制摘要
        是认知注册与 Go 回调共享的唯一哈希计算（CR-LAY-10，CR-GW-09a）。
        """
        ...