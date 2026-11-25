// Package domain 承载 Alert 的纯业务规则：状态机、业务不变量、判定函数与稳定错误码。
//
// 约束：
//   - 不依赖数据库、网络、gin/grpc/pgx、internal/system。
//   - 只表达“给定输入返回决策”，持久化与副作用留在应用层/基础设施层。
//   - 错误统一带稳定 Code，便于映射到 HTTP/内部协议，不得伪装为 INTERNAL。
//   - 测试命名 TestINVxx_<行为>，每个用例注释指向 spec 条款。
package domain
