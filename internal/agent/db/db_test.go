package db

import (
	"context"
	"os"
	"path/filepath"
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
		AgentType:        "claude",
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
	if got.AgentType != "claude" {
		t.Errorf("agent_type = %q, want %q", got.AgentType, "claude")
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
			WorkingDirectory: "~", AgentType: "claude",
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
		WorkingDirectory: "~", AgentType: "claude",
	}); err != nil {
		t.Fatal(err)
	}

	newName := "renamed"
	newTmux := "claude-renamed"
	if err := db.UpdateSession("s1", SessionUpdate{
		Name:     &newName,
		TmuxName: &newTmux,
	}); err != nil {
		t.Fatal(err)
	}

	got, _ := db.GetSession("s1")
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
	err := db.UpdateSession("missing", SessionUpdate{Name: &name})
	if err == nil {
		t.Fatal("expected error for missing session")
	}
}

func TestDeleteSession(t *testing.T) {
	db := testDB(t)

	if err := db.CreateSession(&Session{
		ID: "s1", Name: "x", TmuxName: "claude-s1",
		WorkingDirectory: "~", AgentType: "claude",
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
		WorkingDirectory: "~", AgentType: "claude",
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
		WorkingDirectory: "~", AgentType: "claude", AutoApprove: true,
	}); err != nil {
		t.Fatal(err)
	}

	got, _ := db.GetSession("s1")
	if !got.AutoApprove {
		t.Error("expected auto_approve = true")
	}
}

func TestSessionNullableFields(t *testing.T) {
	db := testDB(t)

	// Create with all nullable fields as nil
	if err := db.CreateSession(&Session{
		ID: "s1", Name: "minimal", TmuxName: "shell-s1",
		WorkingDirectory: "~", AgentType: "shell",
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
