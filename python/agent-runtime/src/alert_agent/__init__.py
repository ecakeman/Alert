"""Alert cognitive runtime package (agent-runtime).

Single five-layer package per design.md §2.1: domain / application /
infrastructure / transport / bootstrap, wired by bootstrap; configuration is
declared centrally under ``alert_agent.config``. Layers land task by task
(T01~T24, see .codeartsdoer/specs/cognitive-rebuild/tasks.md); this root module
carries the package marker only and stays free of runtime imports.
"""

__all__ = ["__version__"]

__version__ = "0.1.0"