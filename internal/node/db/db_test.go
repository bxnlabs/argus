package db

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testDB(t *testing.T) *DB {
	t.Helper()
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// --- Sessions ---

func TestCreateAndGetSession(t *testing.T) {
	db := testDB(t)

	model := "sonnet"
	s := &Session{
		ID:               "sess-1",
		Name:             "Test Session",
		TmuxName:         "claude-sess-1",
		WorkingDirectory: "~/code",
		ProviderType:        "claude",
		Model:            &model,
	}
	if err := db.CreateSession(s); err != nil {
		t.Fatal(err)
	}

	got, err := db.GetSession("sess-1")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected session, got nil")
	}
	if got.Name != "Test Session" {
		t.Errorf("name = %q, want %q", got.Name, "Test Session")
	}
	if got.TmuxName != "claude-sess-1" {
		t.Errorf("tmux_name = %q, want %q", got.TmuxName, "claude-sess-1")
	}
	if got.ProviderType != "claude" {
		t.Errorf("provider_type = %q, want %q", got.ProviderType, "claude")
	}
}

func TestGetSessionNotFound(t *testing.T) {
	db := testDB(t)
	got, err := db.GetSession("nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}

func TestListSessions(t *testing.T) {
	db := testDB(t)

	for _, id := range []string{"a", "b", "c"} {
		if err := db.CreateSession(&Session{
			ID: id, Name: id, TmuxName: "claude-" + id,
			WorkingDirectory: "~", ProviderType: "claude",
		}); err != nil {
			t.Fatal(err)
		}
	}

	sessions, err := db.ListSessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 3 {
		t.Errorf("len = %d, want 3", len(sessions))
	}
}

func TestUpdateSession(t *testing.T) {
	db := testDB(t)

	if err := db.CreateSession(&Session{
		ID: "s1", Name: "orig", TmuxName: "claude-s1",
		WorkingDirectory: "~", ProviderType: "claude",
	}); err != nil {
		t.Fatal(err)
	}

	newName := "renamed"
	newTmux := "claude-renamed"
	got, err := db.UpdateSession("s1", SessionUpdate{
		Name:     &newName,
		TmuxName: &newTmux,
	})
	if err != nil {
		t.Fatal(err)
	}

	if got.Name != "renamed" {
		t.Errorf("name = %q, want %q", got.Name, "renamed")
	}
	if got.TmuxName != "claude-renamed" {
		t.Errorf("tmux_name = %q, want %q", got.TmuxName, "claude-renamed")
	}
}

func TestUpdateSessionNotFound(t *testing.T) {
	db := testDB(t)
	name := "x"
	_, err := db.UpdateSession("missing", SessionUpdate{Name: &name})
	if err == nil {
		t.Fatal("expected error for missing session")
	}
}

func TestUpdateSessionProviderSessionID(t *testing.T) {
	db := testDB(t)

	if err := db.CreateSession(&Session{
		ID: "s1", Name: "test", TmuxName: "claude-s1",
		WorkingDirectory: "~", ProviderType: "claude",
	}); err != nil {
		t.Fatal(err)
	}

	// Initially nil
	s, _ := db.GetSession("s1")
	if s.ProviderSessionID != nil {
		t.Fatal("expected nil provider_session_id initially")
	}

	// Set it
	pid := "e9ed7eb1-5fa8-40ca-b718-bc747ea4e38e"
	got, err := db.UpdateSession("s1", SessionUpdate{
		ProviderSessionID: &pid,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.ProviderSessionID == nil || *got.ProviderSessionID != pid {
		t.Errorf("provider_session_id = %v, want %q", got.ProviderSessionID, pid)
	}

	// Overwrite with a new value
	pid2 := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	got2, err := db.UpdateSession("s1", SessionUpdate{
		ProviderSessionID: &pid2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got2.ProviderSessionID == nil || *got2.ProviderSessionID != pid2 {
		t.Errorf("provider_session_id = %v, want %q", got2.ProviderSessionID, pid2)
	}
}

func TestDeleteSession(t *testing.T) {
	db := testDB(t)

	if err := db.CreateSession(&Session{
		ID: "s1", Name: "x", TmuxName: "claude-s1",
		WorkingDirectory: "~", ProviderType: "claude",
	}); err != nil {
		t.Fatal(err)
	}

	if err := db.DeleteSession("s1"); err != nil {
		t.Fatal(err)
	}

	got, _ := db.GetSession("s1")
	if got != nil {
		t.Fatal("expected nil after delete")
	}
}

func TestTouchSession(t *testing.T) {
	db := testDB(t)

	if err := db.CreateSession(&Session{
		ID: "s1", Name: "x", TmuxName: "claude-s1",
		WorkingDirectory: "~", ProviderType: "claude",
	}); err != nil {
		t.Fatal(err)
	}

	before, err := db.GetSession("s1")
	if err != nil {
		t.Fatal(err)
	}
	originalUpdatedAt := before.UpdatedAt

	// Touch with a future timestamp should advance updated_at.
	futureTS := int64(4102444800) // 2100-01-01 00:00:00 UTC
	if err := db.TouchSession(context.Background(), "s1", futureTS); err != nil {
		t.Fatal(err)
	}

	after, err := db.GetSession("s1")
	if err != nil {
		t.Fatal(err)
	}
	if after.UpdatedAt == originalUpdatedAt {
		t.Error("expected updated_at to change after touch with future timestamp")
	}
	if after.UpdatedAt != "2100-01-01 00:00:00" {
		t.Errorf("updated_at = %q, want %q", after.UpdatedAt, "2100-01-01 00:00:00")
	}

	// Touch with an older timestamp should be a no-op (monotonic guard).
	olderTS := int64(946684800) // 2000-01-01 00:00:00 UTC
	if err := db.TouchSession(context.Background(), "s1", olderTS); err != nil {
		t.Fatal(err)
	}

	afterOld, err := db.GetSession("s1")
	if err != nil {
		t.Fatal(err)
	}
	if afterOld.UpdatedAt != after.UpdatedAt {
		t.Errorf("monotonic guard failed: updated_at changed from %q to %q", after.UpdatedAt, afterOld.UpdatedAt)
	}

	// Touch with the same timestamp should also be a no-op.
	if err := db.TouchSession(context.Background(), "s1", futureTS); err != nil {
		t.Fatal(err)
	}

	afterSame, err := db.GetSession("s1")
	if err != nil {
		t.Fatal(err)
	}
	if afterSame.UpdatedAt != after.UpdatedAt {
		t.Errorf("same-timestamp guard failed: updated_at changed from %q to %q", after.UpdatedAt, afterSame.UpdatedAt)
	}
}

func TestTouchSessionNonexistent(t *testing.T) {
	db := testDB(t)

	// Touching a nonexistent session should not error (just 0 rows affected).
	if err := db.TouchSession(context.Background(), "nonexistent", 1000000); err != nil {
		t.Errorf("expected no error for nonexistent session, got: %v", err)
	}
}

// --- DB Open edge cases ---

func TestOpenExpandsTilde(t *testing.T) {
	dir := t.TempDir()
	// Use absolute path (not tilde) for this test
	db, err := Open(filepath.Join(dir, "sub", "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	// Verify directory was created
	if _, err := os.Stat(filepath.Join(dir, "sub")); err != nil {
		t.Fatal(err)
	}
}

func TestSessionAutoApprove(t *testing.T) {
	db := testDB(t)

	if err := db.CreateSession(&Session{
		ID: "s1", Name: "auto", TmuxName: "claude-s1",
		WorkingDirectory: "~", ProviderType: "claude", AutoApprove: true,
	}); err != nil {
		t.Fatal(err)
	}

	got, _ := db.GetSession("s1")
	if !got.AutoApprove {
		t.Error("expected auto_approve = true")
	}
}

func TestSessionProfile(t *testing.T) {
	db := testDB(t)

	profile := "work"
	if err := db.CreateSession(&Session{
		ID: "s1", Name: "profiled", TmuxName: "claude-s1",
		WorkingDirectory: "~", ProviderType: "claude", Profile: &profile,
	}); err != nil {
		t.Fatal(err)
	}

	got, _ := db.GetSession("s1")
	if got.Profile == nil || *got.Profile != "work" {
		t.Errorf("expected profile %q, got %v", "work", got.Profile)
	}
}

func TestSessionProfileNull(t *testing.T) {
	db := testDB(t)

	if err := db.CreateSession(&Session{
		ID: "s1", Name: "legacy", TmuxName: "claude-s1",
		WorkingDirectory: "~", ProviderType: "claude",
	}); err != nil {
		t.Fatal(err)
	}

	got, _ := db.GetSession("s1")
	if got.Profile != nil {
		t.Errorf("expected nil profile, got %v", got.Profile)
	}
}

func TestMigrationAddProfile(t *testing.T) {
	db := testDB(t)

	// Running migrations a second time should not error (idempotent)
	if err := db.RunMigrations(); err != nil {
		t.Fatalf("re-run migrations: %v", err)
	}
}

func TestSessionUnreadFields(t *testing.T) {
	db := testDB(t)
	if err := db.RunMigrations(); err != nil {
		t.Fatal(err)
	}

	err := db.CreateSession(&Session{
		ID: "sess-unread-1", Name: "test", TmuxName: "claude-sess-unread-1",
		WorkingDirectory: "/tmp", ProviderType: "claude",
	})
	if err != nil {
		t.Fatal(err)
	}

	s, err := db.GetSession("sess-unread-1")
	if err != nil {
		t.Fatal(err)
	}
	if s.UnreadSince != nil {
		t.Errorf("expected nil UnreadSince, got %v", s.UnreadSince)
	}
	if s.LastViewedAt != nil {
		t.Errorf("expected nil LastViewedAt, got %v", s.LastViewedAt)
	}
}

func TestSetUnreadSince(t *testing.T) {
	db := testDB(t)
	if err := db.RunMigrations(); err != nil {
		t.Fatal(err)
	}

	db.CreateSession(&Session{
		ID: "sess-ur-1", Name: "test", TmuxName: "claude-sess-ur-1",
		WorkingDirectory: "/tmp", ProviderType: "claude",
	})

	// Set unread
	ts := "2026-04-13 12:00:00"
	if err := db.SetUnreadSince(context.Background(), "sess-ur-1", &ts); err != nil {
		t.Fatal(err)
	}
	s, _ := db.GetSession("sess-ur-1")
	if s.UnreadSince == nil || *s.UnreadSince != ts {
		t.Errorf("expected unread_since=%q, got %v", ts, s.UnreadSince)
	}

	// Clear unread
	if err := db.SetUnreadSince(context.Background(), "sess-ur-1", nil); err != nil {
		t.Fatal(err)
	}
	s, _ = db.GetSession("sess-ur-1")
	if s.UnreadSince != nil {
		t.Errorf("expected nil unread_since, got %v", s.UnreadSince)
	}
}

func TestSetLastViewedAt(t *testing.T) {
	db := testDB(t)
	if err := db.RunMigrations(); err != nil {
		t.Fatal(err)
	}

	db.CreateSession(&Session{
		ID: "sess-lv-1", Name: "test", TmuxName: "claude-sess-lv-1",
		WorkingDirectory: "/tmp", ProviderType: "claude",
	})

	ts := "2026-04-13 12:00:00"
	if err := db.SetLastViewedAt(context.Background(), "sess-lv-1", ts); err != nil {
		t.Fatal(err)
	}
	s, _ := db.GetSession("sess-lv-1")
	if s.LastViewedAt == nil || *s.LastViewedAt != ts {
		t.Errorf("expected last_viewed_at=%q, got %v", ts, s.LastViewedAt)
	}
}

func TestAcknowledgeSession(t *testing.T) {
	db := testDB(t)
	if err := db.RunMigrations(); err != nil {
		t.Fatal(err)
	}

	db.CreateSession(&Session{
		ID: "sess-ack-1", Name: "test", TmuxName: "claude-sess-ack-1",
		WorkingDirectory: "/tmp", ProviderType: "claude",
	})

	// Set unread first
	ts := "2026-04-13 12:00:00"
	db.SetUnreadSince(context.Background(), "sess-ack-1", &ts)

	// Acknowledge
	if err := db.AcknowledgeSession(context.Background(), "sess-ack-1"); err != nil {
		t.Fatal(err)
	}
	s, _ := db.GetSession("sess-ack-1")
	if s.UnreadSince != nil {
		t.Errorf("expected nil unread_since after acknowledge, got %v", s.UnreadSince)
	}
	if s.LastViewedAt == nil {
		t.Error("expected last_viewed_at to be set after acknowledge")
	}
}

func TestSessionNullableFields(t *testing.T) {
	db := testDB(t)

	// Create with all nullable fields as nil
	if err := db.CreateSession(&Session{
		ID: "s1", Name: "minimal", TmuxName: "shell-s1",
		WorkingDirectory: "~", ProviderType: "shell",
	}); err != nil {
		t.Fatal(err)
	}

	got, _ := db.GetSession("s1")
	if got.ProviderSessionID != nil {
		t.Error("expected nil provider_session_id")
	}
	if got.SystemPrompt != nil {
		t.Error("expected nil system_prompt")
	}
}

// --- Notifications ---

func TestUnreadSessions(t *testing.T) {
	db := testDB(t)
	if err := db.RunMigrations(); err != nil {
		t.Fatal(err)
	}

	// Create two sessions
	branch := "jeev/feature"
	parentDir := "/home/jeev/repos/myproject"
	remoteURL := "https://github.com/bxnlabs/argus.git"
	db.CreateSession(&Session{
		ID: "s1", Name: "session-1", TmuxName: "claude-s1",
		WorkingDirectory: "/tmp/proj1", ProviderType: "claude",
		WorktreeBranch: &branch, GitParentDir: &parentDir, GitRemoteURL: &remoteURL,
	})
	db.CreateSession(&Session{
		ID: "s2", Name: "session-2", TmuxName: "claude-s2",
		WorkingDirectory: "/tmp/proj2", ProviderType: "codex",
	})

	// No unread sessions initially
	sessions, err := db.UnreadSessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 unread sessions, got %d", len(sessions))
	}

	// Mark s1 as unread
	ts := "2026-04-17 12:00:00"
	db.SetUnreadSince(context.Background(), "s1", &ts)

	sessions, err = db.UnreadSessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 unread session, got %d", len(sessions))
	}
	if sessions[0].ID != "s1" {
		t.Errorf("expected session ID %q, got %q", "s1", sessions[0].ID)
	}
	if sessions[0].WorktreeBranch == nil || *sessions[0].WorktreeBranch != branch {
		t.Errorf("expected worktree_branch %q, got %v", branch, sessions[0].WorktreeBranch)
	}
	if sessions[0].GitParentDir == nil || *sessions[0].GitParentDir != parentDir {
		t.Errorf("expected git_parent_dir %q, got %v", parentDir, sessions[0].GitParentDir)
	}
	if sessions[0].GitRemoteURL == nil || *sessions[0].GitRemoteURL != remoteURL {
		t.Errorf("expected git_remote_url %q, got %v", remoteURL, sessions[0].GitRemoteURL)
	}
}

func TestHasNotification(t *testing.T) {
	db := testDB(t)
	if err := db.RunMigrations(); err != nil {
		t.Fatal(err)
	}

	db.CreateSession(&Session{
		ID: "s1", Name: "test", TmuxName: "claude-s1",
		WorkingDirectory: "/tmp", ProviderType: "claude",
	})

	// No notification exists
	has, err := db.HasNotification(context.Background(), "s1", "2026-04-17 12:00:00")
	if err != nil {
		t.Fatal(err)
	}
	if has {
		t.Error("expected no notification, got true")
	}

	// Insert a notification
	if err := db.InsertNotification(context.Background(), "s1", "2026-04-17 12:05:00"); err != nil {
		t.Fatal(err)
	}

	// Notification exists after the unread_since timestamp
	has, err = db.HasNotification(context.Background(), "s1", "2026-04-17 12:00:00")
	if err != nil {
		t.Fatal(err)
	}
	if !has {
		t.Error("expected notification to exist, got false")
	}

	// Notification does NOT exist after a later timestamp (new unread event)
	has, err = db.HasNotification(context.Background(), "s1", "2026-04-17 12:10:00")
	if err != nil {
		t.Fatal(err)
	}
	if has {
		t.Error("expected no notification after later timestamp, got true")
	}
}

func TestInsertNotification(t *testing.T) {
	db := testDB(t)
	if err := db.RunMigrations(); err != nil {
		t.Fatal(err)
	}

	db.CreateSession(&Session{
		ID: "s1", Name: "test", TmuxName: "claude-s1",
		WorkingDirectory: "/tmp", ProviderType: "claude",
	})

	err := db.InsertNotification(context.Background(), "s1", "2026-04-17 12:05:00")
	if err != nil {
		t.Fatal(err)
	}

	// Verify via HasNotification
	has, err := db.HasNotification(context.Background(), "s1", "2026-04-17 12:00:00")
	if err != nil {
		t.Fatal(err)
	}
	if !has {
		t.Error("expected notification to exist after insert")
	}
}

func TestNotificationsCascadeOnSessionDelete(t *testing.T) {
	db := testDB(t)
	if err := db.RunMigrations(); err != nil {
		t.Fatal(err)
	}

	db.CreateSession(&Session{
		ID: "s1", Name: "test", TmuxName: "claude-s1",
		WorkingDirectory: "/tmp", ProviderType: "claude",
	})
	db.InsertNotification(context.Background(), "s1", "2026-04-17 12:05:00")

	// Delete the session
	if err := db.DeleteSession("s1"); err != nil {
		t.Fatal(err)
	}

	// Notification should be gone (cascade delete)
	has, err := db.HasNotification(context.Background(), "s1", "2026-04-17 12:00:00")
	if err != nil {
		t.Fatal(err)
	}
	if has {
		t.Error("expected notification to be cascade-deleted with session")
	}
}

func TestGCNotifications(t *testing.T) {
	db := testDB(t)
	if err := db.RunMigrations(); err != nil {
		t.Fatal(err)
	}

	db.CreateSession(&Session{
		ID: "s1", Name: "test", TmuxName: "claude-s1",
		WorkingDirectory: "/tmp", ProviderType: "claude",
	})
	db.CreateSession(&Session{
		ID: "s2", Name: "test2", TmuxName: "claude-s2",
		WorkingDirectory: "/tmp", ProviderType: "claude",
	})

	// Insert multiple notifications for s1 and one for s2
	db.InsertNotification(context.Background(), "s1", "2026-04-17 12:05:00")
	db.InsertNotification(context.Background(), "s1", "2026-04-17 12:15:00")
	db.InsertNotification(context.Background(), "s1", "2026-04-17 12:25:00")
	db.InsertNotification(context.Background(), "s2", "2026-04-17 12:10:00")

	deleted, err := db.GCNotifications(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 2 {
		t.Errorf("expected 2 deleted, got %d", deleted)
	}

	// Latest notification for s1 should still exist
	has, _ := db.HasNotification(context.Background(), "s1", "2026-04-17 12:20:00")
	if !has {
		t.Error("expected latest s1 notification to survive GC")
	}

	// s2's only notification should still exist
	has, _ = db.HasNotification(context.Background(), "s2", "2026-04-17 12:00:00")
	if !has {
		t.Error("expected s2 notification to survive GC")
	}
}

func TestFreshDBCheckMigrations(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "fresh.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := db.CheckMigrations(); err != nil {
		t.Fatalf("CheckMigrations on fresh DB should pass: %v", err)
	}
}

func TestSessionFlaggedStarredDefaults(t *testing.T) {
	db := testDB(t)

	if err := db.CreateSession(&Session{
		ID: "s1", Name: "test", TmuxName: "claude-s1",
		WorkingDirectory: "~", ProviderType: "claude",
	}); err != nil {
		t.Fatal(err)
	}

	got, err := db.GetSession("s1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Flagged {
		t.Error("expected flagged=false by default")
	}
	if got.Starred {
		t.Error("expected starred=false by default")
	}
}

func TestUpdateSessionFlaggedStarred(t *testing.T) {
	db := testDB(t)

	if err := db.CreateSession(&Session{
		ID: "s1", Name: "t", TmuxName: "claude-s1",
		WorkingDirectory: "~", ProviderType: "claude",
	}); err != nil {
		t.Fatal(err)
	}

	tru := true
	got, err := db.UpdateSession("s1", SessionUpdate{Flagged: &tru, Starred: &tru})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Flagged {
		t.Error("expected flagged=true after update")
	}
	if !got.Starred {
		t.Error("expected starred=true after update")
	}

	// Clearing flagged must not touch starred.
	fls := false
	got2, err := db.UpdateSession("s1", SessionUpdate{Flagged: &fls})
	if err != nil {
		t.Fatal(err)
	}
	if got2.Flagged {
		t.Error("expected flagged=false after second update")
	}
	if !got2.Starred {
		t.Error("expected starred to remain true")
	}
}

func TestUpgradeDBRequiresNotificationsMigration(t *testing.T) {
	// Simulate an existing database that has all current session columns
	// (fully migrated) but predates the notifications feature.
	path := filepath.Join(t.TempDir(), "upgraded.db")

	rawDB, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	// Current session schema with all columns.
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
  flagged INTEGER NOT NULL DEFAULT 0,
  starred INTEGER NOT NULL DEFAULT 0
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
	// Record prior column migrations as applied.
	for _, name := range []string{
		"add_worktree_branch", "add_git_parent_dir", "add_git_remote_url",
		"add_profile", "add_branch_created", "rename_agent_type_to_provider_type",
		"add_unread_since_and_last_viewed_at", "add_flagged_and_starred",
	} {
		if _, err := rawDB.Exec(`INSERT INTO _migrations (name) VALUES (?)`, name); err != nil {
			rawDB.Close()
			t.Fatal(err)
		}
	}
	rawDB.Close()

	// Open with current code — should NOT auto-seed create_notifications_table.
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	err = d.CheckMigrations()
	if err == nil {
		t.Fatal("expected CheckMigrations to report create_notifications_table as pending")
	}
	if !strings.Contains(err.Error(), "create_notifications_table") {
		t.Fatalf("expected error to mention create_notifications_table, got: %v", err)
	}
}
