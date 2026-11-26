// Package gen 持有 protoc 生成的 gRPC 代码，禁止手改。
//
// 生成依赖：protoc 27.3、protoc-gen-go v1.36.10、protoc-gen-go-grpc v1.5.1。
// 在 services/system-core 目录执行 go generate ./internal/transport/grpc/gen 重新生成。
//
//go:generate sh -c "protoc -I ../../../../../../contracts/proto --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative alertflow/system/v1/common.proto alertflow/system/v1/task_gateway.proto alertflow/agent/v1/agent_task.proto"
package gen
