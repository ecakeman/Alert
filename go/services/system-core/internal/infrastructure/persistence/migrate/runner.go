package migrate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ecakeman/alert/services/system-core/migrations"
)

// Migration 是一个已发现的迁移文件。
type Migration struct {
	Version  int64
	Name     string
	Path     string
	SQL      string
	Checksum string
}

// Conflict 描述已应用记录与当前文件的不一致。
type Conflict struct {
	Version  int64
	Applied  string
	Expected string
}

// Result 是一次 Up 执行的结果。
type Result struct {
	Applied   []int64
	Skipped   []int64
	Conflicts []Conflict
}

var nameRe = regexp.MustCompile(`^(\d{4})_([a-z0-9_]+)\.sql$`)

// Discover 读取根目录下的 *.sql 迁移，按版本升序返回。
func Discover(fsys fs.FS) ([]Migration, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("read migrations dir: %w", err)
	}
	var files []Migration
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		m, err := parseFile(fsys, e.Name())
		if err != nil {
			return nil, err
		}
		files = append(files, m)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Version < files[j].Version })
	if err := Validate(files); err != nil {
		return nil, err
	}
	return files, nil
}

func parseFile(fsys fs.FS, name string) (Migration, error) {
	m := nameRe.FindStringSubmatch(name)
	if m == nil {
		return Migration{}, fmt.Errorf("migration filename invalid: %s", name)
	}
	v, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil {
		return Migration{}, fmt.Errorf("migration version invalid: %s", name)
	}
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return Migration{}, fmt.Errorf("read migration %s: %w", name, err)
	}
	if strings.TrimSpace(string(data)) == "" {
		return Migration{}, fmt.Errorf("migration %s is empty", name)
	}
	sum := sha256.Sum256(data)
	return Migration{
		Version:  v,
		Name:     m[2],
		Path:     name,
		SQL:      string(data),
		Checksum: hex.EncodeToString(sum[:]),
	}, nil
}

// Validate 校验重复版本、命名非法与空内容。
func Validate(files []Migration) error {
	seen := map[int64]string{}
	for _, f := range files {
		if !nameRe.MatchString(f.Path) {
			return fmt.Errorf("migration filename invalid: %s", f.Path)
		}
		if strings.TrimSpace(f.SQL) == "" {
			return fmt.Errorf("migration %s is empty", f.Path)
		}
		if prev, ok := seen[f.Version]; ok {
			return fmt.Errorf("duplicate migration version %d: %s and %s", f.Version, prev, f.Path)
		}
		seen[f.Version] = f.Path
	}
	return nil
}

// Compare 比对已应用 checksum 与当前文件；文件缺失或 checksum 不一致即冲突。
func Compare(applied map[int64]string, files []Migration) []Conflict {
	byVersion := map[int64]string{}
	for _, f := range files {
		byVersion[f.Version] = f.Checksum
	}
	var conflicts []Conflict
	for v, appliedChecksum := range applied {
		expected, ok := byVersion[v]
		if !ok {
			conflicts = append(conflicts, Conflict{Version: v, Applied: appliedChecksum, Expected: ""})
			continue
		}
		if expected != appliedChecksum {
			conflicts = append(conflicts, Conflict{Version: v, Applied: appliedChecksum, Expected: expected})
		}
	}
	sort.Slice(conflicts, func(i, j int) bool { return conflicts[i].Version < conflicts[j].Version })
	return conflicts
}

// ExpectedVersion 返回文件集的最大版本；空列表为 0。
func ExpectedVersion(files []Migration) int64 {
	var max int64
	for _, f := range files {
		if f.Version > max {
			max = f.Version
		}
	}
	return max
}

const advisoryLockKey = "alert.system.migrate"

// EnsureVersionTable 幂等引导 system.schema_migrations。
func EnsureVersionTable(ctx context.Context, c *pgx.Conn) error {
	if _, err := c.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS system"); err != nil {
		return fmt.Errorf("ensure schema: %w", err)
	}
	if _, err := c.Exec(ctx, `CREATE TABLE IF NOT EXISTS system.schema_migrations (
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

// Up 在真实 PG 上执行 forward-only 迁移：advisory lock 防并发、
// 单文件单事务、失败只回滚当前文件；存在冲突时不执行任何文件。
func Up(ctx context.Context, pool *pgxpool.Pool) (Result, error) {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("acquire connection: %w", err)
	}
	defer conn.Release()
	c := conn.Conn()

	if _, err := c.Exec(ctx, "SET lock_timeout = '10s'"); err != nil {
		return Result{}, fmt.Errorf("set lock_timeout: %w", err)
	}
	if _, err := c.Exec(ctx, "SELECT pg_advisory_lock(hashtext($1))", advisoryLockKey); err != nil {
		return Result{}, fmt.Errorf("advisory lock: %w", err)
	}
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = c.Exec(unlockCtx, "SELECT pg_advisory_unlock(hashtext($1))", advisoryLockKey)
	}()

	if err := EnsureVersionTable(ctx, c); err != nil {
		return Result{}, err
	}

	files, err := Discover(migrations.FS)
	if err != nil {
		return Result{}, err
	}

	applied := map[int64]string{}
	rows, err := c.Query(ctx, "SELECT version, checksum FROM system.schema_migrations")
	if err != nil {
		return Result{}, fmt.Errorf("read applied migrations: %w", err)
	}
	for rows.Next() {
		var v int64
		var ck string
		if err := rows.Scan(&v, &ck); err != nil {
			rows.Close()
			return Result{}, fmt.Errorf("scan applied migration: %w", err)
		}
		applied[v] = ck
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return Result{}, fmt.Errorf("iterate applied migrations: %w", err)
	}

	conflicts := Compare(applied, files)
	if len(conflicts) > 0 {
		return Result{Conflicts: conflicts}, nil
	}

	var res Result
	for _, f := range files {
		if _, ok := applied[f.Version]; ok {
			res.Skipped = append(res.Skipped, f.Version)
			continue
		}
		start := time.Now()
		tx, err := c.Begin(ctx)
		if err != nil {
			return res, fmt.Errorf("begin migration %d: %w", f.Version, err)
		}
		if _, err := tx.Exec(ctx, f.SQL); err != nil {
			_ = tx.Rollback(ctx)
			return res, fmt.Errorf("migration %d failed: %w", f.Version, err)
		}
		if _, err := tx.Exec(ctx,
			"INSERT INTO system.schema_migrations(version, name, checksum, execution_ms) VALUES ($1,$2,$3,$4)",
			f.Version, f.Name, f.Checksum, time.Since(start).Milliseconds()); err != nil {
			_ = tx.Rollback(ctx)
			return res, fmt.Errorf("record migration %d: %w", f.Version, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return res, fmt.Errorf("commit migration %d: %w", f.Version, err)
		}
		res.Applied = append(res.Applied, f.Version)
	}
	return res, nil
}

// Status 返回已应用最大版本与期望版本；版本表不存在时 applied=0。
func Status(ctx context.Context, pool *pgxpool.Pool) (int64, int64, error) {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("acquire connection: %w", err)
	}
	defer conn.Release()
	c := conn.Conn()

	var regClass *string
	if err := c.QueryRow(ctx, "SELECT to_regclass('system.schema_migrations')::text").Scan(&regClass); err != nil {
		return 0, 0, fmt.Errorf("check version table: %w", err)
	}

	files, err := Discover(migrations.FS)
	if err != nil {
		return 0, 0, err
	}
	expected := ExpectedVersion(files)

	if regClass == nil {
		return 0, expected, nil
	}
	var applied int64
	if err := c.QueryRow(ctx, "SELECT coalesce(max(version),0) FROM system.schema_migrations").Scan(&applied); err != nil {
		return 0, 0, fmt.Errorf("read applied version: %w", err)
	}
	return applied, expected, nil
}