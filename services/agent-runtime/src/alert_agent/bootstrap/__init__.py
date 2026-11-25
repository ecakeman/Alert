"""Bootstrap 层：装配、生命周期、运行角色与入口（CR-BOOT-04~07）。

角色装配拆分在 ``roles`` 中（grpc/http/worker），唯一的 lifespan 统筹启动/
关闭顺序（``lifespan``），``app`` 暴露 FastAPI 应用工厂和进程入口，
``migrate`` 是认知迁移命令入口。application/transport 不得向上导入本包的
任何内容（CR-LAY-05）。
"""

from alert_agent.bootstrap.app import create_app, main

__all__ = ["create_app", "main"]