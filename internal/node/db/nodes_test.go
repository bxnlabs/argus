package db

import (
	"path/filepath"
	"testing"
)

func TestNodesTablePresentOnFreshDB(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer d.Close()

	// Fresh DB must not report pending migrations (add_nodes_table seeded).
	if err := d.CheckMigrations(); err != nil {
		t.Fatalf("CheckMigrations on fresh DB: %v", err)
	}

	var count int
	if err := d.sql.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='nodes'`,
	).Scan(&count); err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}
	if count != 1 {
		t.Errorf("nodes table count = %d, want 1", count)
	}
}
