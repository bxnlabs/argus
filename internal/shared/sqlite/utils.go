package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/bxnlabs/argus/internal/shared"
	_ "modernc.org/sqlite"
)

// Open opens a SQLite database with standard pragmas (WAL, FK, busy timeout).
func Open(path string) (*sql.DB, error) {
	// Expand ~ to home directory
	path, err := shared.ExpandPath(path)
	if err != nil {
		return nil, err
	}

	// Ensure parent directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("mkdir: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}

	// Bound the pool. SQLite serializes writes, so a large pool just
	// wastes file descriptors. 2 connections allows concurrent reads
	// while a write is in progress (WAL mode).
	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(2)

	// SQLite pragmas are per-connection. With a connection pool, new
	// connections won't inherit pragmas. Force-initialize both pool
	// connections so all members have correct settings.
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA foreign_keys=ON",
		"PRAGMA synchronous=NORMAL",
	}
	for i := 0; i < 2; i++ {
		conn, err := db.Conn(context.Background())
		if err != nil {
			db.Close()
			return nil, fmt.Errorf("init conn %d: %w", i, err)
		}
		for _, p := range pragmas {
			if _, err := conn.ExecContext(context.Background(), p); err != nil {
				conn.Close()
				db.Close()
				return nil, fmt.Errorf("pragma %q on conn %d: %w", p, i, err)
			}
		}
		conn.Close() // returns to pool
	}

	return db, nil
}
