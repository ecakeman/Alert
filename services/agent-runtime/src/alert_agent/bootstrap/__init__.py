"""Bootstrap layer: assembly, lifecycle, run roles and entrypoints (CR-BOOT-04~07).

Role wiring is split across ``roles`` (grpc/http/worker), the single lifespan
orchestrates startup/shutdown order (``lifespan``), ``app`` exposes the
FastAPI application factory and the process entrypoint, and ``migrate`` is
the cognitive-migration command entry. Nothing in this package may be
imported by application/transport upward (CR-LAY-05).
"""

from alert_agent.bootstrap.app import create_app, main

__all__ = ["create_app", "main"]