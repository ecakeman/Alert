"""Single route-registration entry for the HTTP transport (CR-LAY-04).

All HTTP routes of the runtime are registered here and only here; handlers
stay thin (binding + basic validation + use-case call + DTO mapping).
"""

from fastapi import APIRouter

from alert_agent.transport.http.handler import health


def build_router() -> APIRouter:
    router = APIRouter()
    router.add_api_route("/health", health.health, methods=["GET"])
    router.add_api_route("/ready", health.ready, methods=["GET"])
    return router