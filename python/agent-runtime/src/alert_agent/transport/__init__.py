"""传输层：协议绑定（HTTP 路由、gRPC servicer、SSE）。

传输层只负责绑定、基础校验、用例调用、DTO 映射与响应写出；业务规则和 SQL 留在
应用层（CR-LAY-04）。禁止导入基础设施实现或 bootstrap。
"""