"""配置层：运行时唯一的 pydantic-settings 入口。

``load_settings`` 返回在启动时校验的冻结 ``Settings`` 树；
``ConfigError`` 携带脱敏的、仅含变量名的失败报告
（CR-BOOT-01~03）。
"""

from alert_agent.config.settings import ConfigError, Settings, load_settings

__all__ = ["ConfigError", "Settings", "load_settings"]