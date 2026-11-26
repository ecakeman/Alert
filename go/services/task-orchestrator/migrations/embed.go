// Package migrations 内嵌 task 域迁移脚本。
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS