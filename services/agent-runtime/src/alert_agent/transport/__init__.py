"""Transport layer: protocol bindings (HTTP routes, gRPC servicer, SSE).

Transport performs binding, basic validation, use-case calls, DTO mapping
and response writing only; business rules and SQL stay in application
(CR-LAY-04). It must not import infrastructure implementations or bootstrap.
"""