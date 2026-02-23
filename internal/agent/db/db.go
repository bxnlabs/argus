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

	return d, nil
}

// Close closes the database.
func (d *DB) Close() error {
	return d.sql.Close()
}
