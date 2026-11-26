"""agent-runtime 的 gRPC 适配器。

``grpc/gen`` 存放由唯一契约源 ``contracts/proto/`` 经
``tools/generate_proto.py`` 再生成的 Python 桩；生成文件已提交入库但绝不
手工编辑（CR-GW-01，CR-DATA-06）。客户端/服务端适配器与 ``gen`` 并列，
由各自的任务负责添加（T06/T07）。
"""