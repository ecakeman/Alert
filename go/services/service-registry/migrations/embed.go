package migrations

import "embed"

// FS 是本域 forward-only 迁移；SQL 随二进制打包，保证"镜像版本 = 迁移版本"。
//
//go:embed *.sql
var FS embed.FS
