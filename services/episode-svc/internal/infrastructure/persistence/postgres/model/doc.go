// Package model 承载 episode schema 行模型：完整列映射、Scan/ToApp/FromApp。
// 行模型是仓储的私有事实，不进入 application 端口。
package model

// Scannable 同时被 pgx.Row 与 pgx.Rows 满足；Scan 的错误（含 pgx.ErrNoRows）由仓储统一 Translate。
type Scannable interface {
	Scan(dest ...any) error
}