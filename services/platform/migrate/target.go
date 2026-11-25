package migrate

import (
	"context"
	"fmt"
	"io/fs"
	"regexp"

	"github.com/jackc/pgx/v5"
)

// Target 描述一个域的迁移目标：独立 schema、独立版本表、独立 advisory lock。
type Target struct {
	Schema       string
	VersionTable string
	LockKey      string
	FS           fs.FS
}

// TargetError 描述迁移目标配置错误；只携带字段名与原因。
type TargetError struct {
	Field  string
	Reason string
}

func (e *TargetError) Error() string { return "migrate target " + e.Field + ": " + e.Reason }

var (
	schemaRe       = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)
	versionTableRe = regexp.MustCompile(`^[a-z_][a-z0-9_]*\.[a-z_][a-z0-9_]*$`)
)

// Validate 校验目标参数，防止未经校验的标识符进入 SQL 拼接。
func (t Target) Validate() error {
	if !schemaRe.MatchString(t.Schema) {
		return &TargetError{Field: "Schema", Reason: "invalid identifier"}
	}
	if !versionTableRe.MatchString(t.VersionTable) {
		return &TargetError{Field: "VersionTable", Reason: "must be schema.table"}
	}
	schema, _ := splitVersionTable(t.VersionTable)
	if schema != t.Schema {
		return &TargetError{Field: "VersionTable", Reason: "schema mismatch"}
	}
	if t.LockKey == "" {
		return &TargetError{Field: "LockKey", Reason: "must not be empty"}
	}
	if t.FS == nil {
		return &TargetError{Field: "FS", Reason: "must not be nil"}
	}
	return nil
}

func splitVersionTable(full string) (schema, table string) {
	for i := 0; i < len(full); i++ {
		if full[i] == '.' {
			return full[:i], full[i+1:]
		}
	}
	return full, ""
}

// EnsureVersionTable 幂等引导 <schema>.<version_table>，列结构与历史 system 表一致。
func EnsureVersionTable(ctx context.Context, c *pgx.Conn, t Target) error {
	if _, err := c.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS "+t.Schema); err != nil {
		return fmt.Errorf("ensure schema: %w", err)
	}
	if _, err := c.Exec(ctx, `CREATE TABLE IF NOT EXISTS `+t.VersionTable+` (
		version bigint PRIMARY KEY,
		name text NOT NULL,
		checksum text NOT NULL,
		applied_at timestamptz NOT NULL DEFAULT now(),
		execution_ms bigint NOT NULL DEFAULT 0
	)`); err != nil {
		return fmt.Errorf("ensure version table: %w", err)
	}
	return nil
}
