//go:build integration

package database_test

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/capyrpi/api/internal/database"
	"github.com/capyrpi/api/internal/testutils"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunMigrations_AppliesInitAndIsIdempotent(t *testing.T) {
	connStr := testutils.SetupTestPostgres(t)
	migrationsPath := testMigrationsPath(t)
	ctx := context.Background()

	require.NoError(t, database.RunMigrations(ctx, connStr, migrationsPath))
	require.NoError(t, database.RunMigrations(ctx, connStr, migrationsPath))

	pool, err := database.NewPool(ctx, connStr)
	require.NoError(t, err)
	defer pool.Close()

	for _, table := range []string{
		"users",
		"organizations",
		"org_members",
		"events",
		"event_hosting",
		"event_registrations",
		"bot_tokens",
	} {
		var regclass pgtype.Text
		err := pool.QueryRow(ctx, "SELECT to_regclass($1)::text", "public."+table).Scan(&regclass)
		require.NoError(t, err)
		assert.Truef(t, regclass.Valid, "expected table %s to exist", table)
	}

	var version int64
	var dirty bool
	err = pool.QueryRow(ctx, "SELECT version, dirty FROM schema_migrations").Scan(&version, &dirty)
	require.NoError(t, err)
	assert.Equal(t, int64(1), version)
	assert.False(t, dirty)
}

func TestRunMigrations_DownAndUp(t *testing.T) {
	connStr := testutils.SetupTestPostgres(t)
	migrationsPath := testMigrationsPath(t)
	ctx := context.Background()

	require.NoError(t, database.RunMigrations(ctx, connStr, migrationsPath))
	require.NoError(t, database.RunMigrationsDown(ctx, connStr, migrationsPath, 1))

	pool, err := database.NewPool(ctx, connStr)
	require.NoError(t, err)

	var regclass pgtype.Text
	err = pool.QueryRow(ctx, "SELECT to_regclass('public.users')::text").Scan(&regclass)
	require.NoError(t, err)
	assert.False(t, regclass.Valid, "users table should be removed after rolling back init migration")
	pool.Close()

	require.NoError(t, database.RunMigrations(ctx, connStr, migrationsPath))

	pool, err = database.NewPool(ctx, connStr)
	require.NoError(t, err)
	defer pool.Close()

	err = pool.QueryRow(ctx, "SELECT to_regclass('public.users')::text").Scan(&regclass)
	require.NoError(t, err)
	assert.True(t, regclass.Valid, "users table should exist after re-applying migrations")
}

func testMigrationsPath(t *testing.T) string {
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok)

	projectRoot := filepath.Join(filepath.Dir(filename), "../..")
	return filepath.Join(projectRoot, "migrations")
}
