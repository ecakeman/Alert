"""Observability adapter (T23): bounded queue + background batched flush."""

from alert_agent.infrastructure.observe.sink import BoundedObservationSink

__all__ = ["BoundedObservationSink"]