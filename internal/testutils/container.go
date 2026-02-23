package testutils

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/capyrpi/api/internal/database"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// SetupTestPostgres creates a fresh Postgres container and returns its connection string.
func SetupTestPostgres(t *testing.T) string {
	ctx := context.Background()

	pgContainer, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("test_db"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second)),
	)
	if err != nil {
		t.Fatalf("failed to start postgres container: %v", err)
	}

	t.Cleanup(func() {
		if err := pgContainer.Terminate(ctx); err != nil {
			t.Fatalf("failed to terminate container: %v", err)
		}
	})

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("failed to get connection string: %v", err)
	}

	return connStr
}

// SetupTestDB creates a fresh Postgres container, applies migrations, and returns the connection pool.
func SetupTestDB(t *testing.T) *pgxpool.Pool {
	ctx := context.Background()
	connStr := SetupTestPostgres(t)

	// Get repo root for migrations/
	_, filename, _, _ := runtime.Caller(0)
	projectRoot := filepath.Join(filepath.Dir(filename), "../..")
	migrationsPath := filepath.Join(projectRoot, "migrations")

	if err := database.RunMigrations(ctx, connStr, migrationsPath); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	pool, err := database.NewPool(ctx, connStr)
	if err != nil {
		t.Fatalf("failed to connect to database: %v", err)
	}

	return pool
}

// SetupEmptyTestDB creates a fresh Postgres container without loading schema.sql.
// Use this for migration tests that need to apply schema changes from scratch.
func SetupEmptyTestDB(t *testing.T) *pgxpool.Pool {
	ctx := context.Background()

	pgContainer, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("test_db"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second)),
	)
	if err != nil {
		t.Fatalf("failed to start postgres container: %v", err)
	}

	t.Cleanup(func() {
		if err := pgContainer.Terminate(ctx); err != nil {
			t.Fatalf("failed to terminate container: %v", err)
		}
	})

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("failed to get connection string: %v", err)
	}

	pool, err := database.NewPool(ctx, connStr)
	if err != nil {
		t.Fatalf("failed to connect to database: %v", err)
	}

	return pool
}
