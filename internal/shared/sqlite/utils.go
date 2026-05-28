package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
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

	// SQLite pragmas are per-connection (busy_timeout, foreign_keys and
	// synchronous are not persisted in the file). Pass them in the DSN so the
	// modernc driver re-applies them to every connection the pool opens —
	// including ones created lazily under concurrency. Configuring connections
	// manually after sql.Open misses those, leaving them at busy_timeout=0,
	// which returns SQLITE_BUSY immediately on contention instead of waiting.
	params := url.Values{}
	params.Add("_pragma", "busy_timeout(5000)")
	params.Add("_pragma", "journal_mode(WAL)")
	params.Add("_pragma", "foreign_keys(on)")
	params.Add("_pragma", "synchronous(normal)")
	// Build a file: URI rather than concatenating path + "?" + query: the
	// modernc driver splits a plain path on the first '?', so a path
	// containing one would truncate the filename and silently drop the
	// pragmas. A file: URI percent-encodes the path, which SQLite decodes.
	dsn := (&url.URL{Scheme: "file", OmitHost: true, Path: path, RawQuery: params.Encode()}).String()

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}

	// Bound the pool. SQLite serializes writes, so a large pool just
	// wastes file descriptors. 2 connections allows concurrent reads
	// while a write is in progress (WAL mode).
	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(2)

	// sql.Open is lazy and never opens a connection, so a bad path or
	// invalid pragma would otherwise surface on first query. Force one
	// connection to fail fast; the DSN still applies pragmas to every
	// pool connection opened later.
	if err := db.PingContext(context.Background()); err != nil {
		db.Close()
		return nil, fmt.Errorf("open: %w", err)
	}

	return db, nil
}
