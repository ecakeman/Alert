"""Single route-registration entry for the HTTP transport (CR-LAY-04).

All HTTP routes of the runtime are registered here and only here; handlers
stay thin (binding + basic validation + use-case call + DTO mapping). RAG
protocol routes declare the standalone internal-token dependency themselves
(CR-REG-51).
"""

from fastapi import APIRouter, Depends

from alert_agent.transport.http.deps import require_internal_token
from alert_agent.transport.http.handler import health, ingest, search


def build_router() -> APIRouter:
    router = APIRouter()
    router.add_api_route("/health", health.health, methods=["GET"])
    router.add_api_route("/ready", health.ready, methods=["GET"])
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