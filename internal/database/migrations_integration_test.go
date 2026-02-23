//go:build integration

package database_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/capyrpi/api/internal/testutils"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func TestMigrationsApplyAndRollback(t *testing.T) {
	migrationsDir := repoRoot(t, 2)
	migrationsDir = filepath.Join(migrationsDir, "migrations")

	upFiles, downFiles := migrationFiles(t, migrationsDir)
	if len(upFiles) == 0 {
		t.Skipf("no migration files found in %s", migrationsDir)
	}
	require.Equal(t, len(upFiles), len(downFiles), "up/down migration file count mismatch")

	pool := testutils.SetupEmptyTestDB(t)
	defer pool.Close()

	ctx := context.Background()

	beforeState := snapshotPublicSchema(t, ctx, pool)

	for i, path := range upFiles {
		sqlBytes, err := os.ReadFile(path)
		require.NoError(t, err)
		_, err = pool.Exec(ctx, string(sqlBytes))
		require.NoErrorf(t, err, "failed applying up migration %d: %s", i, filepath.Base(path))
	}

	afterUpState := snapshotPublicSchema(t, ctx, pool)
	require.NotEqual(t, beforeState, afterUpState, "expected schema to change after applying up migrations")

	for i := len(downFiles) - 1; i >= 0; i-- {
		path := downFiles[i]
		sqlBytes, err := os.ReadFile(path)
		require.NoError(t, err)
		_, err = pool.Exec(ctx, string(sqlBytes))
		require.NoErrorf(t, err, "failed applying down migration %d: %s", i, filepath.Base(path))
	}

	afterDownState := snapshotPublicSchema(t, ctx, pool)
	require.Equal(t, beforeState, afterDownState, "expected schema state after down migrations to match initial state")
}

func migrationFiles(t *testing.T, dir string) ([]string, []string) {
	t.Helper()

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)

	var upFiles []string
	var downFiles []string

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		fullPath := filepath.Join(dir, name)

		switch {
		case strings.HasSuffix(name, ".up.sql"):
			upFiles = append(upFiles, fullPath)
		case strings.HasSuffix(name, ".down.sql"):
			downFiles = append(downFiles, fullPath)
		}
	}

	sort.Strings(upFiles)
	sort.Strings(downFiles)
	return upFiles, downFiles
}

func repoRoot(t *testing.T, upLevels int) string {
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok)

	root := filepath.Dir(filename)
	for range upLevels {
		root = filepath.Dir(root)
	}
	return root
}

type schemaSnapshot struct {
	Tables    []string
	Columns   []string
	Indexes   []string
	Views     []string
	Sequences []string
	Enums     []string
}

func snapshotPublicSchema(t *testing.T, ctx context.Context, pool *pgxpool.Pool) schemaSnapshot {
	t.Helper()

	return schemaSnapshot{
		Tables:    querySingleColumn(t, ctx, pool, `SELECT table_name FROM information_schema.tables WHERE table_schema = 'public' AND table_type = 'BASE TABLE' ORDER BY table_name`),
		Columns:   queryColumns(t, ctx, pool),
		Indexes:   querySingleColumn(t, ctx, pool, `SELECT indexname || ':' || indexdef FROM pg_indexes WHERE schemaname = 'public' ORDER BY indexname`),
		Views:     querySingleColumn(t, ctx, pool, `SELECT table_name FROM information_schema.views WHERE table_schema = 'public' ORDER BY table_name`),
		Sequences: querySingleColumn(t, ctx, pool, `SELECT sequence_name FROM information_schema.sequences WHERE sequence_schema = 'public' ORDER BY sequence_name`),
		Enums:     queryEnums(t, ctx, pool),
	}
}

func querySingleColumn(t *testing.T, ctx context.Context, pool *pgxpool.Pool, sql string) []string {
	t.Helper()

	rows, err := pool.Query(ctx, sql)
	require.NoError(t, err)
	defer rows.Close()

	var out []string
	for rows.Next() {
		var v string
		require.NoError(t, rows.Scan(&v))
		out = append(out, v)
	}
	require.NoError(t, rows.Err())
	return out
}

func queryColumns(t *testing.T, ctx context.Context, pool *pgxpool.Pool) []string {
	t.Helper()

	rows, err := pool.Query(ctx, `
		SELECT table_name, column_name, data_type, is_nullable, COALESCE(column_default, '')
		FROM information_schema.columns
		WHERE table_schema = 'public'
		ORDER BY table_name, ordinal_position
	`)
	require.NoError(t, err)
	defer rows.Close()

	var out []string
	for rows.Next() {
		var tableName, columnName, dataType, isNullable, columnDefault string
		require.NoError(t, rows.Scan(&tableName, &columnName, &dataType, &isNullable, &columnDefault))
		out = append(out, fmt.Sprintf("%s.%s:%s:%s:%s", tableName, columnName, dataType, isNullable, columnDefault))
	}
	require.NoError(t, rows.Err())
	return out
}

func queryEnums(t *testing.T, ctx context.Context, pool *pgxpool.Pool) []string {
	t.Helper()

	rows, err := pool.Query(ctx, `
		SELECT t.typname, e.enumlabel
		FROM pg_type t
		JOIN pg_enum e ON e.enumtypid = t.oid
		JOIN pg_namespace n ON n.oid = t.typnamespace
		WHERE n.nspname = 'public'
		ORDER BY t.typname, e.enumsortorder
	`)
	require.NoError(t, err)
	defer rows.Close()

	var out []string
	for rows.Next() {
		var typeName, label string
		require.NoError(t, rows.Scan(&typeName, &label))
		out = append(out, fmt.Sprintf("%s:%s", typeName, label))
	}
	require.NoError(t, rows.Err())
	return out
}
