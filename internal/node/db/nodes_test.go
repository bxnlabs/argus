package db

import (
	"database/sql"
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

func TestMigrationAddsNodesTableToExistingDB(t *testing.T) {
	// Simulate an existing DB that predates the nodes table. We create the
	// sessions and _migrations tables and record every migration that would
	// have been applied before add_nodes_table was introduced. This ensures:
	//   - priorMigrations > 0, so seedMigrations does NOT auto-mark
	//     add_nodes_table as applied (the seeding gate is
	//     priorMigrations==0 && allColumnsPresent).
	//   - The nodes table is genuinely absent from the raw schema.
	path := filepath.Join(t.TempDir(), "pre-nodes.db")

	rawDB, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rawDB.Exec(`
CREATE TABLE IF NOT EXISTS sessions (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  tmux_name TEXT NOT NULL UNIQUE,
  created_at TEXT NOT NULL DEFAULT (datetime('now')),
  updated_at TEXT NOT NULL DEFAULT (datetime('now')),
  working_directory TEXT NOT NULL DEFAULT '~',
  provider_session_id TEXT,
  model TEXT DEFAULT 'sonnet',
  system_prompt TEXT,
  provider_type TEXT NOT NULL DEFAULT 'claude',
  auto_approve INTEGER NOT NULL DEFAULT 0,
  worktree_branch TEXT,
  git_parent_dir TEXT,
  git_remote_url TEXT,
  profile TEXT,
  branch_created INTEGER NOT NULL DEFAULT 0,
  unread_since TEXT,
  last_viewed_at TEXT,
  pinned INTEGER NOT NULL DEFAULT 0,
  user_marked_unread_at TEXT
);
CREATE TABLE IF NOT EXISTS _migrations (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL UNIQUE,
  applied_at TEXT NOT NULL DEFAULT (datetime('now'))
);`)
	if err != nil {
		rawDB.Close()
		t.Fatal(err)
	}
	// Record all migrations that predate add_nodes_table as applied.
	for _, name := range []string{
		"add_worktree_branch", "add_git_parent_dir", "add_git_remote_url",
		"add_profile", "add_branch_created", "rename_agent_type_to_provider_type",
		"add_unread_since_and_last_viewed_at", "create_notifications_table",
		"add_pinned", "add_user_marked_unread_at",
	} {
		if _, err := rawDB.Exec(`INSERT INTO _migrations (name) VALUES (?)`, name); err != nil {
			rawDB.Close()
			t.Fatal(err)
		}
	}
	rawDB.Close()

	// Open with current code — add_nodes_table must remain pending (not seeded).
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	if err := d.RunMigrations(); err != nil {
		t.Fatalf("RunMigrations should create the nodes table: %v", err)
	}

	var count int
	if err := d.sql.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='nodes'`,
	).Scan(&count); err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}
	if count != 1 {
		t.Errorf("nodes table count = %d, want 1 after migration", count)
	}
}
