package sqlite

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

// TestAllPoolConnsHaveBusyTimeout verifies every connection the pool can hand
// out has busy_timeout configured. busy_timeout is per-connection and is NOT
// persisted in the database file, so an unconfigured connection defaults to 0
// and returns SQLITE_BUSY immediately on contention instead of waiting.
func TestAllPoolConnsHaveBusyTimeout(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()

	// Hold both connections open simultaneously to force the pool to
	// materialize all MaxOpenConns physical connections.
	c1, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer c1.Close()
	c2, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer c2.Close()

	for i, c := range []*sql.Conn{c1, c2} {
		var timeout int
		if err := c.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&timeout); err != nil {
			t.Fatalf("conn %d: query busy_timeout: %v", i, err)
		}
		if timeout != 5000 {
			t.Errorf("conn %d: busy_timeout = %d, want 5000", i, timeout)
		}

		var journal string
		if err := c.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journal); err != nil {
			t.Fatalf("conn %d: query journal_mode: %v", i, err)
		}
		if journal != "wal" {
			t.Errorf("conn %d: journal_mode = %q, want %q", i, journal, "wal")
		}

		var fk int
		if err := c.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fk); err != nil {
			t.Fatalf("conn %d: query foreign_keys: %v", i, err)
		}
		if fk != 1 {
			t.Errorf("conn %d: foreign_keys = %d, want 1", i, fk)
		}
	}
}

// TestOpenPathWithQuestionMark guards DSN construction: the modernc driver
// splits a plain path on the first '?', so a literal '?' in the path would
// truncate the filename (opening the wrong file) and drop the pragmas. The
// file: URI built by Open must percent-encode the path instead.
func TestOpenPathWithQuestionMark(t *testing.T) {
	path := filepath.Join(t.TempDir(), "weird?name.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	var timeout int
	if err := db.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&timeout); err != nil {
		t.Fatalf("query busy_timeout: %v", err)
	}
	if timeout != 5000 {
		t.Errorf("busy_timeout = %d, want 5000", timeout)
	}

	// The database file must be created at the literal path, not a name
	// truncated at the '?'.
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected db file at %q: %v", path, err)
	}
}
