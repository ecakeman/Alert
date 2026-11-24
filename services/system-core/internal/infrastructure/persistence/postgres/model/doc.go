// Package model 承载 system schema 行模型：完整列映射、Scan/ToApp/FromApp。
// 行模型是仓储的私有事实，不进入 application 端口（端口类型精简见 S1.9）。
// SQL 与错误翻译仍留在仓储层。
package model

// Scannable 同时被 pgx.Row 与 pgx.Rows 满足；Scan 的错误（含 pgx.ErrNoRows）由仓储统一 Translate。
type Scannable interface {
	Scan(dest ...any) error
}
