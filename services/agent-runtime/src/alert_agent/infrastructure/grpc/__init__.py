"""gRPC adapters for the agent-runtime.

``grpc/gen`` holds the Python stubs regenerated from the single contract
source ``contracts/proto/`` via ``tools/generate_proto.py``; generated files
are committed but never hand-edited (CR-GW-01, CR-DATA-06). Client/servicer
adapters live beside ``gen`` and are added by their owning tasks (T06/T07).
"""