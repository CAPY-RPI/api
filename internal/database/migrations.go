package database

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// RunMigrations applies all pending migrations from the given path.
func RunMigrations(ctx context.Context, databaseURL, migrationsPath string) error {
	_ = ctx

	m, err := newMigrator(databaseURL, migrationsPath)
	if err != nil {
		return err
	}
	defer closeMigrator(m)

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}

	return nil
}

// RunMigrationsDown rolls back a positive number of migration steps.
func RunMigrationsDown(ctx context.Context, databaseURL, migrationsPath string, steps int) error {
	_ = ctx
	if steps <= 0 {
		return fmt.Errorf("steps must be greater than 0")
	}

	m, err := newMigrator(databaseURL, migrationsPath)
	if err != nil {
		return err
	}
	defer closeMigrator(m)

	if err := m.Steps(-steps); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("failed to roll back migrations: %w", err)
	}

	return nil
}

func newMigrator(databaseURL, migrationsPath string) (*migrate.Migrate, error) {
	sourceURL, err := fileSourceURL(migrationsPath)
	if err != nil {
		return nil, err
	}

	m, err := migrate.New(sourceURL, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create migrator: %w", err)
	}

	return m, nil
}

func fileSourceURL(path string) (string, error) {
	if strings.Contains(path, "://") {
		return path, nil
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("failed to resolve migrations path %q: %w", path, err)
	}

	return "file://" + filepath.ToSlash(absPath), nil
}

func closeMigrator(m *migrate.Migrate) {
	sourceErr, dbErr := m.Close()
	if err := errors.Join(sourceErr, dbErr); err != nil {
		// Best-effort cleanup; callers care about migration result more than close errors.
		return
	}
}
