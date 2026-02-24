package cli

import "testing"

func TestResolveSession_ExactName(t *testing.T) {
	sessions := []sessionInfo{
		{ID: "sess_abc", Name: "my-session", TmuxName: "claude-sess_abc"},
		{ID: "sess_def", Name: "other", TmuxName: "claude-sess_def"},
	}
	s, err := resolveSession(sessions, "my-session")
	if err != nil {
		t.Fatalf("resolveSession: %v", err)
	}
	if s.ID != "sess_abc" {
		t.Errorf("id = %q, want %q", s.ID, "sess_abc")
	}
}

func TestResolveSession_IDPrefix(t *testing.T) {
	sessions := []sessionInfo{
		{ID: "sess_abc123_xyz", Name: "my-session", TmuxName: "claude-sess_abc123_xyz"},
		{ID: "sess_def456_uvw", Name: "other", TmuxName: "claude-sess_def456_uvw"},
	}
	s, err := resolveSession(sessions, "sess_abc")
	if err != nil {
		t.Fatalf("resolveSession: %v", err)
	}
	if s.ID != "sess_abc123_xyz" {
		t.Errorf("id = %q, want %q", s.ID, "sess_abc123_xyz")
	}
}

func TestResolveSession_Ambiguous(t *testing.T) {
	sessions := []sessionInfo{
		{ID: "sess_abc1", Name: "session-a"},
		{ID: "sess_abc2", Name: "session-b"},
	}
	_, err := resolveSession(sessions, "sess_abc")
	if err == nil {
		t.Fatal("expected error for ambiguous match")
	}
}

func TestResolveSession_NoMatch(t *testing.T) {
	sessions := []sessionInfo{
		{ID: "sess_abc", Name: "my-session"},
	}
	_, err := resolveSession(sessions, "nonexistent")
	if err == nil {
		t.Fatal("expected error for no match")
	}
}
