package db_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/bxnlabs/argus/internal/agent/db"
	_ "modernc.org/sqlite"
)

func openTestDB(t *testing.T) *db.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	d, err := db.Open(path)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	if err := d.RunMigrations(); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	return d
}

func TestWorktreeBranchColumn(t *testing.T) {
	d := openTestDB(t)

	branch := "jeev/fix-auth"
	s := &db.Session{
		ID:               "sess_test_1",
		Name:             "fix auth",
		TmuxName:         "claude-sess_test_1",
		WorkingDirectory: "/tmp/wt",
		AgentType:        "claude",
		WorktreeBranch:   &branch,
	}

	if err := d.CreateSession(s); err != nil {
		t.Fatalf("create session: %v", err)
	}

	got, err := d.GetSession("sess_test_1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if got.WorktreeBranch == nil {
		t.Fatal("expected WorktreeBranch to be set")
	}
	if *got.WorktreeBranch != branch {
		t.Errorf("expected WorktreeBranch %q, got %q", branch, *got.WorktreeBranch)
	}
}

func TestWorktreeBranchNullable(t *testing.T) {
	d := openTestDB(t)

	s := &db.Session{
		ID:               "sess_test_2",
		Name:             "plain session",
		TmuxName:         "claude-sess_test_2",
		WorkingDirectory: "/tmp/plain",
		AgentType:        "claude",
	}

	if err := d.CreateSession(s); err != nil {
		t.Fatalf("create session: %v", err)
	}

	got, err := d.GetSession("sess_test_2")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if got.WorktreeBranch != nil {
		t.Errorf("expected nil WorktreeBranch, got %q", *got.WorktreeBranch)
	}
}

func TestMigrationsIdempotent(t *testing.T) {
	d := openTestDB(t)
	// Running migrations a second time should not error
	if err := d.RunMigrations(); err != nil {
		t.Fatalf("second RunMigrations failed: %v", err)
	}
}

func TestMigrationsUpgradesExistingDB(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")

	// Create a DB with the old schema (no worktree_branch column).
	oldSchema := `
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
  agent_type TEXT NOT NULL DEFAULT 'claude',
  auto_approve INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS _migrations (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL UNIQUE,
  applied_at TEXT NOT NULL DEFAULT (datetime('now'))
);`

	rawDB, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open raw db: %v", err)
	}
	if _, err := rawDB.Exec(oldSchema); err != nil {
		rawDB.Close()
		t.Fatalf("create old schema: %v", err)
	}
	rawDB.Close()

	// Now open it with the current db.Open (which will run seedMigrations).
	d, err := db.Open(path)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer d.Close()

	// RunMigrations must add the column.
	if err := d.RunMigrations(); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	// Verify the column exists by creating a session with a worktree branch.
	branch := "jeev/upgrade-test"
	s := &db.Session{
		ID:               "sess_upgrade",
		Name:             "upgrade test",
		TmuxName:         "claude-sess_upgrade",
		WorkingDirectory: "/tmp/upgrade",
		AgentType:        "claude",
		WorktreeBranch:   &branch,
	}
	if err := d.CreateSession(s); err != nil {
		t.Fatalf("create session after migration: %v", err)
	}

	got, err := d.GetSession("sess_upgrade")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if got.WorktreeBranch == nil || *got.WorktreeBranch != branch {
		t.Errorf("expected WorktreeBranch %q, got %v", branch, got.WorktreeBranch)
	}
}
