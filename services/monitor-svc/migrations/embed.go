// Package migrations 内嵌 monitor 域迁移脚本。
package migrations

import "embed"

// FS 是 monitor 域 SQL 迁移文件集合（NNNN_name.sql）。
//
//go:embed *.sql
var FS embed.FS