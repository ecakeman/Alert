// Package migrations 内嵌 episode 域迁移脚本。
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
