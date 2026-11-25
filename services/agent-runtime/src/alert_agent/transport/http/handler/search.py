"""POST /search 处理器（T18，CR-LAY-04）。

只做绑定 + 基础校验 + 用例调用 + DTO 映射 + 响应写出：无业务规则、无 SQL、
无编排分支。领域错误映射为标准 HTTP 状态码，并在响应体中携带业务码
（CR-REG-52）：FatalError -> 400，RetryableError -> 503。
"""

from __future__ import annotations

from fastapi import HTTPException, Request

from alert_agent.domain.shared import FatalError, RetryableError
from alert_agent.transport.http.dto.search import (
    SearchRequestDto,
    SearchResponseDto,
)
from alert_agent.transport.http.mapper import response_from, to_search_params


async def search(request: Request, dto: SearchRequestDto) -> SearchResponseDto:
    searcher = getattr(request.app.state, "retrieval_searcher", None)
    if searcher is None:
        raise HTTPException(
            status_code=503,
            detail={"code": "NOT_READY", "message": "retrieval stack is not wired yet"},
        )
    try:
        result = await searcher.search(**to_search_params(dto))
    except FatalError as exc:
        raise HTTPException(
            status_code=400,
            detail={"code": exc.code.value, "message": exc.message},
        ) from exc
    except RetryableError as exc:
        raise HTTPException(
            status_code=503,
            detail={"code": exc.code.value, "message": exc.message},
        ) from exc
    return response_from(result)