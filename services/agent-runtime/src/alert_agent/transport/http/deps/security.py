"""内部令牌鉴权依赖（design 5.3.1，CR-REG-36/51）。

独立的 FastAPI 依赖：每条 RAG 协议路由都必须自行声明对本函数的依赖
——鉴权绝不由其他路由旁路提供（CR-REG-51，规避 RAG B1）。比较为恒定时间；
占位令牌已在启动时被拒绝（T02 fail-fast，R3），因此缺失或为空的配置令牌
在此同样会失败关闭。
"""

from __future__ import annotations

import hmac

from fastapi import HTTPException, Request

_HEADER = "X-Internal-Token"


async def require_internal_token(request: Request) -> None:
    settings = getattr(request.app.state, "settings", None)
    # internal_token 位于 gateway 配置组（T02 占位符检查）。
    configured = getattr(getattr(settings, "gateway", None), "internal_token", "")
    provided = request.headers.get(_HEADER, "")
    if (
        not configured
        or not provided
        or not hmac.compare_digest(provided.encode("utf-8"), configured.encode("utf-8"))
    ):
        raise HTTPException(
            status_code=401,
            detail={"code": "UNAUTHORIZED", "message": "missing or invalid internal token"},
        )