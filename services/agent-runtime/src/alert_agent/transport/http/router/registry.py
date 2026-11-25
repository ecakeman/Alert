"""HTTP 传输层唯一的路由注册入口（CR-LAY-04）。

运行时的所有 HTTP 路由都在此处且仅在此处注册；处理器保持薄层
（绑定 + 基础校验 + 用例调用 + DTO 映射）。RAG 协议路由自行声明独立的
内部令牌依赖（CR-REG-51）。
"""

from fastapi import APIRouter, Depends

from alert_agent.transport.http.deps import require_internal_token
from alert_agent.transport.http.handler import a2a, chat, health, ingest, search


def build_router() -> APIRouter:
    router = APIRouter()
    router.add_api_route("/health", health.health, methods=["GET"])
    router.add_api_route("/ready", health.ready, methods=["GET"])
    router.add_api_route(
        "/.well-known/agent.json",
        a2a.agent_card,
        methods=["GET"],
    )
    router.add_api_route(
        "/a2a/message/send",
        a2a.message_send,
        methods=["POST"],
        dependencies=[Depends(require_internal_token)],
    )
    router.add_api_route(
        "/chat",
        chat.chat,
        methods=["POST"],
        dependencies=[Depends(require_internal_token)],
    )
    router.add_api_route(
        "/search",
        search.search,
        methods=["POST"],
        dependencies=[Depends(require_internal_token)],
    )
    router.add_api_route(
        "/ingest",
        ingest.accept,
        methods=["POST"],
        dependencies=[Depends(require_internal_token)],
    )
    router.add_api_route(
        "/ingest/{doc_id}/status",
        ingest.status,
        methods=["GET"],
        dependencies=[Depends(require_internal_token)],
    )
    return router