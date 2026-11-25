"""可观测性适配器（T23）：有界队列 + 后台批量落库。"""

from alert_agent.infrastructure.observe.sink import BoundedObservationSink

__all__ = ["BoundedObservationSink"]