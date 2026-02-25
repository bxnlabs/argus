package db_test

import (
	"path/filepath"
	"testing"

	"github.com/bxnlabs/argus/internal/agent/db"
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
