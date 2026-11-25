"""Configuration layer: the single pydantic-settings entry for the runtime.

``load_settings`` returns a frozen ``Settings`` tree validated at startup;
``ConfigError`` carries desensitised, variable-name-only failure reports
(CR-BOOT-01~03).
"""

from alert_agent.config.settings import ConfigError, Settings, load_settings

__all__ = ["ConfigError", "Settings", "load_settings"]