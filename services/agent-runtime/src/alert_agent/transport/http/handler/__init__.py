"""HTTP handlers: binding + basic validation + use-case calls + response write.

Handlers carry no business rules and no SQL (CR-LAY-04); they reach runtime
state (e.g. readiness checks) through the injected application state, never
by importing bootstrap.
"""