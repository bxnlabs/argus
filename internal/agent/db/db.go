package db

import (
	"database/sql"
	"fmt"

	"github.com/bxnlabs/argus/internal/shared/sqlite"
)

// DB wraps a SQLite database with Argus-specific operations.
type DB struct {
	sql *sql.DB
}

// Open opens the database and runs schema creation.
func Open(path string) (*DB, error) {
	sqlDB, err := sqlite.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	d := &DB{sql: sqlDB}

	if _, err := d.sql.Exec(schema); err != nil {
		d.sql.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}

	// Seed migrations that are already reflected in the schema so that
	// RunMigrations skips them on fresh databases.
	if err := d.seedMigrations(); err != nil {
		d.sql.Close()
		return nil, fmt.Errorf("seed migrations: %w", err)
	}

	return d, nil
}

// seedMigrations records migrations that are already part of the base schema
// using INSERT OR IGNORE so they are never re-applied on existing databases.
func (d *DB) seedMigrations() error {
	migrations := []string{
		"add_worktree_branch",
	}
	for _, name := range migrations {
		_, err := d.sql.Exec(
			`INSERT OR IGNORE INTO _migrations (name) VALUES (?)`, name,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

// Close closes the database.
func (d *DB) Close() error {
	return d.sql.Close()
}
